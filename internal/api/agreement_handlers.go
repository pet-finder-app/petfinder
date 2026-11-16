package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
)

type agreementTemplateRequest struct {
	Kind string `json:"kind" enum:"adoption,surrender"`
	Body string `json:"body" minLength:"20" maxLength:"50000" doc:"Placeholders: {{user_name}}, {{organization_name}}, {{animal_name}}, {{representative_name}}, {{date}}."`
}
type agreementTemplate struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Version   int       `json:"version"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type templateList struct {
	Items []agreementTemplate `json:"items"`
}
type createAgreementRequest struct {
	TemplateID       string `json:"template_id" format:"uuid"`
	RepresentativeID string `json:"representative_id" format:"uuid"`
}
type agreementSignature struct {
	SignerID string    `json:"signer_id"`
	SHA256   string    `json:"sha256"`
	SignedAt time.Time `json:"signed_at"`
}
type agreementResponse struct {
	ID               string               `json:"id"`
	TemplateID       string               `json:"template_id"`
	Body             string               `json:"body"`
	SHA256           string               `json:"sha256"`
	UserID           string               `json:"user_id"`
	RepresentativeID string               `json:"representative_id"`
	CreatedAt        time.Time            `json:"created_at"`
	Signatures       []agreementSignature `json:"signatures"`
}
type signAgreementRequest struct {
	Code   string `json:"code" pattern:"^[0-9]{6}$" writeOnly:"true"`
	SHA256 string `json:"sha256" pattern:"^[a-f0-9]{64}$"`
	Accept bool   `json:"accept" enum:"true"`
}

func registerAgreementRoutes(s *Server, reg *routeRegistry) {
	registerRoute[agreementTemplateRequest, agreementTemplate](reg, "POST", "/v1/organizations/{organizationId}/agreement-templates", "createAgreementTemplate", "Create a versioned agreement template", "Agreements", 201, s.protected(s.createAgreementTemplate), bearerAuth(false))
	registerRoute[noBody, templateList](reg, "GET", "/v1/organizations/{organizationId}/agreement-templates", "listAgreementTemplates", "List immutable agreement template versions", "Agreements", 200, s.protected(s.listAgreementTemplates), bearerAuth(false))
	registerRoute[createAgreementRequest, agreementResponse](reg, "POST", "/v1/applications/{applicationId}/agreement", "createAdoptionAgreement", "Populate an immutable adoption agreement", "Agreements", 201, s.protected(s.createAgreement), bearerAuth(false))
	registerRoute[noBody, agreementResponse](reg, "GET", "/v1/applications/{applicationId}/agreement", "getAdoptionAgreement", "Find an application's agreement after reload", "Agreements", 200, s.protected(s.getSubjectAgreement), bearerAuth(false))
	registerRoute[noBody, agreementResponse](reg, "GET", "/v1/agreements/{agreementId}", "getAgreement", "Read an agreement and signatures", "Agreements", 200, s.protected(s.getAgreement), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "POST", "/v1/agreements/{agreementId}/signing-code", "sendAgreementCode", "Email a signer a single-use code", "Agreements", 202, s.protected(s.sendAgreementCode), bearerAuth(false))
	registerRoute[signAgreementRequest, agreementResponse](reg, "POST", "/v1/agreements/{agreementId}/signatures", "signAgreement", "Sign the immutable agreement content", "Agreements", 200, s.protected(s.signAgreement), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "GET", "/v1/agreements/{agreementId}/print", "printAgreement", "Read an authorized printable agreement", "Agreements", 200, s.protected(s.printAgreement), bearerAuth(false), printableContract)
}

var agreementPlaceholders = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func validTemplate(body string) bool {
	keys := map[string]bool{"user_name": true, "organization_name": true, "animal_name": true, "representative_name": true, "date": true}
	for _, m := range agreementPlaceholders.FindAllStringSubmatch(body, -1) {
		if !keys[m[1]] {
			return false
		}
	}
	clean := agreementPlaceholders.ReplaceAllString(body, "")
	return !strings.Contains(clean, "{{") && !strings.Contains(clean, "}}")
}
func (s *Server) createAgreementTemplate(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.financeAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req agreementTemplateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validTemplate(req.Body) {
		writeServiceError(w, ErrInvalid)
		return
	}
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	if _, err = tx.Exec(r.Context(), "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", org); err != nil {
		handleDBError(w, err)
		return
	}
	var v agreementTemplate
	err = tx.QueryRow(r.Context(), `INSERT INTO agreement_templates(organization_id,kind,version,body,created_by) SELECT $1,$2,COALESCE(max(version),0)+1,$3,$4 FROM agreement_templates WHERE organization_id=$1 AND kind=$2 RETURNING id::text,kind,version,body,created_at`, org, req.Kind, req.Body, actor).Scan(&v.ID, &v.Kind, &v.Version, &v.Body, &v.CreatedAt)
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
func (s *Server) listAgreementTemplates(w http.ResponseWriter, r *http.Request) {
	org, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	if !s.analyticsAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	rows, err := s.pool.Query(r.Context(), "SELECT id::text,kind,version,body,created_at FROM agreement_templates WHERE organization_id=$1 ORDER BY kind,version DESC", org)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []agreementTemplate{}
	for rows.Next() {
		var v agreementTemplate
		if err = rows.Scan(&v.ID, &v.Kind, &v.Version, &v.Body, &v.CreatedAt); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, templateList{Items: items})
}
func (s *Server) createAgreement(w http.ResponseWriter, r *http.Request) {
	var req createAgreementRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var appID, surrenderID, user, org pgtype.UUID
	var animalName string
	kind := "adoption"
	if r.PathValue("applicationId") != "" {
		var ok bool
		appID, ok = pathUUID(r, "applicationId")
		if !ok {
			invalidPath(w)
			return
		}
		app, err := s.queries.GetAdoptionApplicationByID(r.Context(), appID)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if !s.applicationAccess(r, app, true) {
			writeServiceError(w, ErrForbidden)
			return
		}
		if app.Status != "approved" {
			writeServiceError(w, ErrConflict)
			return
		}
		user, org, animalName = app.AdopterID, app.OrganizationID, app.AnimalName
	} else {
		kind = "surrender"
		var ok bool
		surrenderID, ok = pathUUID(r, "surrenderId")
		if !ok {
			invalidPath(w)
			return
		}
		var status string
		err := s.pool.QueryRow(r.Context(), "SELECT requester_id,organization_id,animal_snapshot->>'name',status FROM surrenders WHERE id=$1", surrenderID).Scan(&user, &org, &animalName, &status)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if !s.analyticsAllowed(r, org) {
			writeServiceError(w, ErrForbidden)
			return
		}
		if status != "accepted" {
			writeServiceError(w, ErrConflict)
			return
		}
	}
	representative, _ := pathUUIDValue(req.RepresentativeID)
	member, active := s.activeMember(r, org, representative)
	if !active || !member.CanManageApplications || representative == user {
		writeServiceError(w, ErrInvalid)
		return
	}
	var template, userName, orgName, repName string
	err := s.pool.QueryRow(r.Context(), `SELECT t.body,u.display_name,o.display_name,rep.display_name FROM agreement_templates t JOIN users u ON u.id=$4 JOIN organizations o ON o.id=t.organization_id JOIN users rep ON rep.id=$5 WHERE t.id=$1 AND t.organization_id=$2 AND t.kind=$3`, req.TemplateID, org, kind, user, representative).Scan(&template, &userName, &orgName, &repName)
	if err != nil {
		handleDBError(w, err)
		return
	}
	body := strings.NewReplacer("{{user_name}}", userName, "{{organization_name}}", orgName, "{{animal_name}}", animalName, "{{representative_name}}", repName, "{{date}}", time.Now().UTC().Format("2006-01-02")).Replace(template)
	hash := sha256.Sum256([]byte(body))
	var id string
	err = s.pool.QueryRow(r.Context(), `INSERT INTO agreements(application_id,surrender_id,organization_id,template_id,user_id,representative_id,body,sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, appID, surrenderID, org, req.TemplateID, user, representative, body, hex.EncodeToString(hash[:])).Scan(&id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	r.SetPathValue("agreementId", id)
	v, ok := s.loadAgreement(w, r)
	if !ok {
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) getSubjectAgreement(w http.ResponseWriter, r *http.Request) {
	var appID, surrenderID pgtype.UUID
	var ok bool
	if r.PathValue("applicationId") != "" {
		appID, ok = pathUUID(r, "applicationId")
	} else {
		surrenderID, ok = pathUUID(r, "surrenderId")
	}
	if !ok {
		invalidPath(w)
		return
	}
	var id string
	if err := s.pool.QueryRow(r.Context(), "SELECT id::text FROM agreements WHERE application_id=$1 OR surrender_id=$2", appID, surrenderID).Scan(&id); err != nil {
		handleDBError(w, err)
		return
	}
	r.SetPathValue("agreementId", id)
	s.getAgreement(w, r)
}
func (s *Server) loadAgreement(w http.ResponseWriter, r *http.Request) (agreementResponse, bool) {
	var v agreementResponse
	id, ok := pathUUID(r, "agreementId")
	if !ok {
		invalidPath(w)
		return v, false
	}
	var org, user, representative pgtype.UUID
	err := s.pool.QueryRow(r.Context(), "SELECT id::text,template_id::text,body,sha256,user_id::text,representative_id::text,created_at,organization_id,user_id,representative_id FROM agreements WHERE id=$1", id).Scan(&v.ID, &v.TemplateID, &v.Body, &v.SHA256, &v.UserID, &v.RepresentativeID, &v.CreatedAt, &org, &user, &representative)
	if err != nil {
		handleDBError(w, err)
		return v, false
	}
	actor, _ := subjectUUID(r)
	if actor != user && actor != representative && !s.analyticsAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return v, false
	}
	v.Signatures = []agreementSignature{}
	rows, err := s.pool.Query(r.Context(), "SELECT signer_id::text,sha256,signed_at FROM agreement_signatures WHERE agreement_id=$1 ORDER BY signed_at,id", id)
	if err != nil {
		handleDBError(w, err)
		return v, false
	}
	defer rows.Close()
	for rows.Next() {
		var signature agreementSignature
		if err = rows.Scan(&signature.SignerID, &signature.SHA256, &signature.SignedAt); err != nil {
			handleDBError(w, err)
			return v, false
		}
		v.Signatures = append(v.Signatures, signature)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return v, false
	}
	return v, true
}
func (s *Server) getAgreement(w http.ResponseWriter, r *http.Request) {
	v, ok := s.loadAgreement(w, r)
	if ok {
		writeJSON(w, 200, v)
	}
}
func signingCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
func (s *Server) sendAgreementCode(w http.ResponseWriter, r *http.Request) {
	v, ok := s.loadAgreement(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	if uuidString(actor) != v.UserID && uuidString(actor) != v.RepresentativeID {
		writeServiceError(w, ErrForbidden)
		return
	}
	if !s.agreementSignable(r, v) {
		writeServiceError(w, ErrConflict)
		return
	}
	if s.email == nil {
		writeProblem(w, 503, "Email unavailable", "email delivery is not configured", nil)
		return
	}
	for _, sig := range v.Signatures {
		if sig.SignerID == uuidString(actor) {
			writeServiceError(w, ErrConflict)
			return
		}
	}
	code, err := signingCode()
	if err != nil {
		handleDBError(w, err)
		return
	}
	hash, err := auth.HashPassword(code)
	if err != nil {
		handleDBError(w, err)
		return
	}
	user, err := s.queries.GetUserByID(r.Context(), actor)
	if err != nil {
		handleDBError(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	tag, err := tx.Exec(r.Context(), `INSERT INTO agreement_codes(agreement_id,signer_id,code_hash,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes') ON CONFLICT(agreement_id,signer_id) DO UPDATE SET code_hash=EXCLUDED.code_hash,expires_at=EXCLUDED.expires_at,failed_attempts=0,used_at=NULL WHERE agreement_codes.expires_at<=now()+interval '9 minutes'`, v.ID, actor, hash)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeProblem(w, 429, "Try again later", "wait one minute before requesting another signing code", nil)
		return
	}
	if err = s.email.Send(r.Context(), user.Email, "PetFinder agreement signing code", "Code for agreement "+v.ID+": "+code+"\nContent SHA256: "+v.SHA256); err != nil {
		writeProblem(w, 503, "Email unavailable", "unable to deliver signing code", nil)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(202)
}
func (s *Server) signAgreement(w http.ResponseWriter, r *http.Request) {
	v, ok := s.loadAgreement(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	if uuidString(actor) != v.UserID && uuidString(actor) != v.RepresentativeID {
		writeServiceError(w, ErrForbidden)
		return
	}
	if !s.agreementSignable(r, v) {
		writeServiceError(w, ErrConflict)
		return
	}
	var req signAgreementRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SHA256 != v.SHA256 {
		writeServiceError(w, ErrConflict)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	var hash string
	var expires time.Time
	var attempts int
	var used *time.Time
	err = tx.QueryRow(r.Context(), "SELECT code_hash,expires_at,failed_attempts,used_at FROM agreement_codes WHERE agreement_id=$1 AND signer_id=$2 FOR UPDATE", v.ID, actor).Scan(&hash, &expires, &attempts, &used)
	if err != nil {
		writeServiceError(w, ErrInvalid)
		return
	}
	if attempts >= 5 || used != nil || !expires.After(time.Now()) {
		writeServiceError(w, ErrConflict)
		return
	}
	if auth.VerifyPassword(hash, req.Code) != nil {
		if _, err = tx.Exec(r.Context(), "UPDATE agreement_codes SET failed_attempts=failed_attempts+1 WHERE agreement_id=$1 AND signer_id=$2", v.ID, actor); err != nil {
			handleDBError(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			handleDBError(w, err)
			return
		}
		writeServiceError(w, ErrInvalid)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO agreement_signatures(agreement_id,signer_id,sha256,ip_address,user_agent) VALUES($1,$2,$3,$4,$5)", v.ID, actor, v.SHA256, r.RemoteAddr, truncate(r.UserAgent(), 500)); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE agreement_codes SET used_at=now() WHERE agreement_id=$1 AND signer_id=$2", v.ID, actor); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getAgreement(w, r)
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func (s *Server) printAgreement(w http.ResponseWriter, r *http.Request) {
	v, ok := s.loadAgreement(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = fmt.Fprintf(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>Agreement</title></head><body><h1>Agreement %s</h1><pre>%s</pre><p>SHA256: %s</p>", html.EscapeString(v.ID), html.EscapeString(v.Body), v.SHA256)
	for _, sig := range v.Signatures {
		_, _ = fmt.Fprintf(w, "<p>Signed by %s at %s</p>", html.EscapeString(sig.SignerID), sig.SignedAt.UTC().Format(time.RFC3339))
	}
	_, _ = fmt.Fprint(w, "</body></html>")
}

func printableContract(_ *routeRegistry, op *huma.Operation) {
	op.Responses["200"].Content = map[string]*huma.MediaType{"text/html": {Schema: &huma.Schema{Type: "string"}}}
}

func (s *Server) agreementSignable(r *http.Request, v agreementResponse) bool {
	var org pgtype.UUID
	var valid bool
	err := s.pool.QueryRow(r.Context(), `SELECT a.organization_id,COALESCE(app.status IN ('approved','completed'),s.status='accepted',false) FROM agreements a LEFT JOIN adoption_applications app ON app.id=a.application_id LEFT JOIN surrenders s ON s.id=a.surrender_id WHERE a.id=$1`, v.ID).Scan(&org, &valid)
	if err != nil || !valid {
		return false
	}
	actor, _ := subjectUUID(r)
	if uuidString(actor) == v.RepresentativeID {
		member, active := s.activeMember(r, org, actor)
		return active && member.CanManageApplications
	}
	return true
}
