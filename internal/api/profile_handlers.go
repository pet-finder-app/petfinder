package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
)

func optionalJSON[T any](value *T) []byte {
	if value == nil {
		return nil
	}
	b, _ := json.Marshal(value)
	return b
}

func validatePersonalProfile(w http.ResponseWriter, p *personalProfile) bool {
	if p.CPF != "" && !validCPF(p.CPF) {
		writeProblem(w, 422, "Validation failed", "invalid CPF checksum", nil)
		return false
	}
	if p.BirthDate != "" {
		date, err := time.Parse("2006-01-02", p.BirthDate)
		if err != nil || date.After(time.Now()) {
			writeServiceError(w, ErrInvalid)
			return false
		}
	}
	links := append(append([]string{}, p.SocialLinks...), p.Documents...)
	if p.AvatarURL != "" {
		links = append(links, p.AvatarURL)
	}
	return validateLinks(w, links)
}
func validateLinks(w http.ResponseWriter, links []string) bool {
	errs := fieldErrors{}
	for _, link := range links {
		validImageURL(errs, "url", link)
	}
	if !errs.empty() {
		writeProblem(w, 422, "Validation failed", "documents and media must use HTTP(S) URLs", errs)
		return false
	}
	return true
}
func validCPF(value string) bool {
	if len(value) != 11 || strings.Count(value, value[:1]) == 11 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	for n := 9; n < 11; n++ {
		sum := 0
		for i := 0; i < n; i++ {
			sum += int(value[i]-'0') * (n + 1 - i)
		}
		digit := (sum * 10) % 11
		if digit == 10 {
			digit = 0
		}
		if digit != int(value[n]-'0') {
			return false
		}
	}
	return true
}

func registerProfileRoutes(s *Server, reg *routeRegistry) {
	registerRoute[noBody, verificationRequirements](reg, "GET", "/v1/me/verification/requirements", "personalVerificationRequirements", "Read personal verification requirements", "Verification", 200, s.protected(s.verificationRequirements), bearerAuth(false))
	registerRoute[noBody, verificationRequirements](reg, "GET", "/v1/organizations/{organizationId}/verification-requirements", "organizationVerificationRequirements", "Read organization verification requirements", "Verification", 200, s.protected(s.verificationRequirements), bearerAuth(false))
	registerRoute[noBody, collectionResponse[verificationReviewItem]](reg, "GET", "/v1/verifications", "listVerificationQueue", "List verification submissions for administrative review", "Verification", 200, s.admin(s.verificationQueue), adminOnly, queryParameters[paginationQuery], queryParameters[verificationStatusQuery])
	registerRoute[noBody, verificationReviewItem](reg, "GET", "/v1/verifications/{verificationId}", "getVerificationReview", "Read submitted evidence and its identity snapshot", "Verification", 200, s.admin(s.getVerificationReview), adminOnly)

	registerRoute[noBody, verificationHistory](reg, "GET", "/v1/me/verification", "getPersonalVerification", "Read personal verification submissions", "Verification", 200, s.protected(s.verificationHistory), bearerAuth(false))
	registerRoute[verificationEvidence, verificationRecord](reg, "POST", "/v1/me/verification", "submitPersonalVerification", "Submit identity evidence for review", "Verification", 201, s.protected(s.submitVerification), bearerAuth(false))
	registerRoute[noBody, verificationHistory](reg, "GET", "/v1/organizations/{organizationId}/verification-submissions", "getOrganizationVerificationHistory", "Read organization verification submissions", "Verification", 200, s.protected(s.verificationHistory), bearerAuth(false))
	registerRoute[verificationEvidence, verificationRecord](reg, "POST", "/v1/organizations/{organizationId}/verification-submissions", "submitOrganizationVerification", "Submit organization verification evidence", "Verification", 201, s.protected(s.submitVerification), bearerAuth(false))
	registerRoute[verificationDecision, verificationRecord](reg, "POST", "/v1/verifications/{verificationId}/decision", "reviewVerification", "Review identity or organization evidence", "Verification", 200, s.admin(s.reviewVerification), adminOnly)
}
func (s *Server) verificationScope(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, bool) {
	actor, _ := subjectUUID(r)
	if r.PathValue("organizationId") == "" {
		return actor, pgtype.UUID{}, true
	}
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	member, allowed := s.activeMember(r, org, actor)
	if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageMembers) {
		writeServiceError(w, ErrForbidden)
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	return pgtype.UUID{}, org, true
}
func (s *Server) verificationHistory(w http.ResponseWriter, r *http.Request) {
	user, org, ok := s.verificationScope(w, r)
	if !ok {
		return
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text,evidence,status,reason,submitted_by::text,reviewed_by::text,created_at,reviewed_at FROM verification_submissions WHERE user_id=$1 OR organization_id=$2 ORDER BY created_at DESC,id`, user, org)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []verificationRecord{}
	for rows.Next() {
		var v verificationRecord
		if err = rows.Scan(&v.ID, &v.Evidence, &v.Status, &v.Reason, &v.SubmittedBy, &v.ReviewedBy, &v.CreatedAt, &v.ReviewedAt); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, verificationHistory{Items: items})
}
func (s *Server) submitVerification(w http.ResponseWriter, r *http.Request) {
	user, org, ok := s.verificationScope(w, r)
	if !ok {
		return
	}
	var request verificationEvidence
	if !decodeJSON(w, r, &request) || !validateLinks(w, request.Documents) {
		return
	}
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	var v verificationRecord
	err = tx.QueryRow(r.Context(), `INSERT INTO verification_submissions(user_id,organization_id,evidence,submitted_by,target_snapshot) VALUES($1,$2,$3,$4,COALESCE((SELECT personal_profile FROM users WHERE id=$1),(SELECT jsonb_build_object('display_name',display_name,'pix_key',(SELECT pix_key FROM organization_payment_details WHERE organization_id=organizations.id),'public_profile',public_profile,'private_profile',private_profile,'legal_name',legal_name,'registration_number',registration_number) FROM organizations WHERE id=$2))) RETURNING id::text,evidence,status,reason,submitted_by::text,reviewed_by::text,created_at,reviewed_at`, user, org, optionalJSON(&request), actor).Scan(&v.ID, &v.Evidence, &v.Status, &v.Reason, &v.SubmittedBy, &v.ReviewedBy, &v.CreatedAt, &v.ReviewedAt)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if user.Valid {
		_, err = tx.Exec(r.Context(), "UPDATE users SET personal_verification_status='pending' WHERE id=$1", user)
	} else {
		// A suspension stands until an administrator reviews the new evidence.
		_, err = tx.Exec(r.Context(), "UPDATE organizations SET verification_status='pending' WHERE id=$1 AND deactivated_at IS NULL AND verification_status<>'suspended'", org)
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) reviewVerification(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "verificationId")
	if !ok {
		invalidPath(w)
		return
	}
	var request verificationDecision
	if !decodeJSON(w, r, &request) {
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	var user, org pgtype.UUID
	var status string
	err = tx.QueryRow(r.Context(), "SELECT user_id,organization_id,status FROM verification_submissions WHERE id=$1 FOR UPDATE", id).Scan(&user, &org, &status)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if status != "submitted" && status != "under_review" {
		writeServiceError(w, ErrConflict)
		return
	}
	if request.Decision == "approved" {
		var unchanged bool
		err = tx.QueryRow(r.Context(), `SELECT target_snapshot=COALESCE((SELECT personal_profile FROM users WHERE id=$2),(SELECT jsonb_build_object('display_name',display_name,'pix_key',(SELECT pix_key FROM organization_payment_details WHERE organization_id=organizations.id),'public_profile',public_profile,'private_profile',private_profile,'legal_name',legal_name,'registration_number',registration_number) FROM organizations WHERE id=$3)) FROM verification_submissions WHERE id=$1`, id, user, org).Scan(&unchanged)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if !unchanged {
			writeProblem(w, 409, "Profile changed", "reject and request new evidence after profile changes", nil)
			return
		}
	}
	actor, _ := subjectUUID(r)
	var v verificationRecord
	err = tx.QueryRow(r.Context(), `UPDATE verification_submissions SET status=$2,reason=$3,reviewed_by=$4,reviewed_at=CASE WHEN $2='under_review' THEN NULL ELSE now() END WHERE id=$1 RETURNING id::text,evidence,status,reason,submitted_by::text,reviewed_by::text,created_at,reviewed_at`, id, request.Decision, request.Reason, actor).Scan(&v.ID, &v.Evidence, &v.Status, &v.Reason, &v.SubmittedBy, &v.ReviewedBy, &v.CreatedAt, &v.ReviewedAt)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if request.Decision != "under_review" {
		result := "rejected"
		if request.Decision == "approved" {
			result = "verified"
		}
		if user.Valid {
			_, err = tx.Exec(r.Context(), "UPDATE users SET personal_verification_status=$2 WHERE id=$1", user, result)
		} else {
			_, err = tx.Exec(r.Context(), `UPDATE organizations SET verification_status=$2,verification_notes=$3,verified_by=CASE WHEN $2='verified' THEN $4 ELSE verified_by END,verified_at=CASE WHEN $2='verified' THEN now() ELSE verified_at END WHERE id=$1`, org, result, request.Reason, actor)
		}
		if err != nil {
			handleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}

const verificationReviewProjection = `jsonb_build_object('verification',jsonb_build_object('id',v.id,'evidence',v.evidence,'status',v.status,'reason',v.reason,'submitted_by',v.submitted_by,'reviewed_by',v.reviewed_by,'created_at',v.created_at,'reviewed_at',v.reviewed_at),'user_id',v.user_id,'organization_id',v.organization_id,'profile',v.target_snapshot)`

func (s *Server) verificationRequirements(w http.ResponseWriter, r *http.Request) {
	_, org, ok := s.verificationScope(w, r)
	if !ok {
		return
	}
	documents := []string{"identity_document"}
	if org.Valid {
		documents = []string{"organization_registration", "representative_identity"}
	}
	writeJSON(w, 200, verificationRequirements{RequiredDocuments: documents, MaximumDocuments: 20})
}
func (s *Server) verificationQueue(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", "submitted", "under_review", "approved", "rejected":
	default:
		writeServiceError(w, ErrInvalid)
		return
	}
	offset, size, page := pagination(r)
	rows, err := s.pool.Query(r.Context(), `SELECT `+verificationReviewProjection+` FROM verification_submissions v WHERE ($1='' OR status=$1) ORDER BY created_at,id LIMIT $2 OFFSET $3`, status, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []verificationReviewItem{}
	for rows.Next() {
		var v verificationReviewItem
		if err = rows.Scan(&v); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	total, ok := s.countRows(w, r, `SELECT count(*) FROM verification_submissions WHERE ($1='' OR status=$1)`, status)
	if !ok {
		return
	}
	page.Total = total
	writeJSON(w, 200, collectionResponse[verificationReviewItem]{Items: items, Page: page})
}
func (s *Server) getVerificationReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "verificationId")
	if !ok {
		invalidPath(w)
		return
	}
	var v verificationReviewItem
	if err := s.pool.QueryRow(r.Context(), `SELECT `+verificationReviewProjection+` FROM verification_submissions v WHERE id=$1`, id).Scan(&v); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
