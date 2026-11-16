package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

type surrenderRequest struct {
	OrganizationID string         `json:"organization_id" format:"uuid"`
	Kind           string         `json:"kind" enum:"intake,return"`
	AnimalID       *string        `json:"animal_id,omitempty" format:"uuid"`
	ApplicationID  *string        `json:"application_id,omitempty" format:"uuid"`
	Animal         *animalRequest `json:"animal,omitempty"`
	Reason         string         `json:"reason" minLength:"20" maxLength:"5000"`
	Documents      []string       `json:"documents,omitempty" maxItems:"20"`
}
type surrenderResubmission struct {
	Animal    *animalRequest `json:"animal,omitempty"`
	Reason    string         `json:"reason" minLength:"20" maxLength:"5000"`
	Documents []string       `json:"documents,omitempty" maxItems:"20"`
}
type surrenderDecision struct {
	Decision string `json:"decision" enum:"accepted,rejected,needs_information"`
	Reason   string `json:"reason" minLength:"5" maxLength:"3000"`
}
type surrenderHandover struct {
	Type string `json:"type" enum:"delivery,receipt"`
	Code string `json:"code,omitempty" pattern:"^[0-9]{6}$" writeOnly:"true"`
}
type surrenderHistoryRecord struct {
	Snapshot  *animalRequest `json:"snapshot,omitempty"`
	Action    string         `json:"action"`
	Notes     string         `json:"notes"`
	ActorID   string         `json:"actor_id"`
	CreatedAt time.Time      `json:"created_at"`
}
type surrenderResponse struct {
	ID             string                   `json:"id"`
	OrganizationID string                   `json:"organization_id"`
	RequesterID    string                   `json:"requester_id"`
	Kind           string                   `json:"kind"`
	AnimalID       *string                  `json:"animal_id"`
	ApplicationID  *string                  `json:"application_id"`
	Animal         animalRequest            `json:"animal"`
	Reason         string                   `json:"reason"`
	Documents      []string                 `json:"documents"`
	Status         string                   `json:"status"`
	ReviewReason   *string                  `json:"review_reason"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	DeliveredAt    *time.Time               `json:"delivered_at"`
	ReceivedAt     *time.Time               `json:"received_at"`
	CompletedAt    *time.Time               `json:"completed_at"`
	AgreementID    *string                  `json:"agreement_id"`
	History        []surrenderHistoryRecord `json:"history"`
}

const surrenderProjection = `jsonb_build_object('id',s.id,'organization_id',s.organization_id,'requester_id',s.requester_id,'kind',s.kind,'animal_id',s.animal_id,'application_id',s.application_id,'animal',s.animal_snapshot,'reason',s.reason,'documents',s.documents,'status',s.status,'review_reason',s.review_reason,'created_at',s.created_at,'updated_at',s.updated_at,'delivered_at',s.delivered_at,'received_at',s.received_at,'completed_at',s.completed_at,'agreement_id',(SELECT id FROM agreements WHERE surrender_id=s.id),'history',COALESCE((SELECT jsonb_agg(jsonb_build_object('action',h.action,'notes',h.notes,'actor_id',h.actor_id,'created_at',h.created_at,'snapshot',h.snapshot) ORDER BY h.id) FROM surrender_history h WHERE h.surrender_id=s.id),'[]'::jsonb))`

func registerSurrenderRoutes(s *Server, reg *routeRegistry) {
	registerRoute[surrenderRequest, surrenderResponse](reg, "POST", "/v1/surrenders", "createSurrender", "Request organization intake or return of an adoption", "Surrenders", 201, s.protected(s.createSurrender), bearerAuth(false))
	registerRoute[noBody, collectionResponse[surrenderResponse]](reg, "GET", "/v1/surrenders", "listSurrenders", "List your intake or organization review cases", "Surrenders", 200, s.protected(s.listSurrenders), bearerAuth(false), queryParameters[paginationQuery], queryParameters[organizationIDQuery])
	registerRoute[noBody, surrenderResponse](reg, "GET", "/v1/surrenders/{surrenderId}", "getSurrender", "Read an intake case and its history", "Surrenders", 200, s.protected(s.getSurrender), bearerAuth(false))
	registerRoute[surrenderResubmission, surrenderResponse](reg, "PUT", "/v1/surrenders/{surrenderId}", "resubmitSurrender", "Resubmit requested intake evidence", "Surrenders", 200, s.protected(s.resubmitSurrender), bearerAuth(false))
	registerRoute[surrenderDecision, surrenderResponse](reg, "POST", "/v1/surrenders/{surrenderId}/decision", "decideSurrender", "Review an intake or return request", "Surrenders", 200, s.protected(s.decideSurrender), bearerAuth(false))
	registerRoute[lifecycleReason, surrenderResponse](reg, "POST", "/v1/surrenders/{surrenderId}/cancellation", "cancelSurrender", "Cancel intake before handover", "Surrenders", 200, s.protected(s.cancelSurrender), bearerAuth(false))
	registerRoute[createAgreementRequest, agreementResponse](reg, "POST", "/v1/surrenders/{surrenderId}/agreement", "createSurrenderAgreement", "Populate an immutable surrender agreement", "Surrenders", 201, s.protected(s.createAgreement), bearerAuth(false))
	registerRoute[noBody, agreementResponse](reg, "GET", "/v1/surrenders/{surrenderId}/agreement", "getSurrenderAgreement", "Read a surrender agreement after reload", "Surrenders", 200, s.protected(s.getSubjectAgreement), bearerAuth(false))
	registerRoute[noBody, noBody](reg, "POST", "/v1/surrenders/{surrenderId}/handover-code", "sendSurrenderCode", "Email the requester a handover confirmation code", "Surrenders", 202, s.protected(s.sendSurrenderCode), bearerAuth(false))
	registerRoute[surrenderHandover, surrenderResponse](reg, "POST", "/v1/surrenders/{surrenderId}/handover", "confirmSurrenderHandover", "Confirm signed surrender delivery or receipt", "Surrenders", 200, s.protected(s.confirmSurrenderHandover), bearerAuth(false))
}
func (s *Server) surrenderScope(w http.ResponseWriter, r *http.Request) (surrenderResponse, bool) {
	var v surrenderResponse
	id, ok := pathUUID(r, "surrenderId")
	if !ok {
		invalidPath(w)
		return v, false
	}
	if err := s.pool.QueryRow(r.Context(), "SELECT "+surrenderProjection+" FROM surrenders s WHERE id=$1", id).Scan(&v); err != nil {
		handleDBError(w, err)
		return v, false
	}
	actor, _ := subjectUUID(r)
	org, _ := pathUUIDValue(v.OrganizationID)
	if uuidString(actor) != v.RequesterID && !s.analyticsAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return v, false
	}
	return v, true
}
func (s *Server) getSurrender(w http.ResponseWriter, r *http.Request) {
	v, ok := s.surrenderScope(w, r)
	if ok {
		writeJSON(w, 200, v)
	}
}
func snapshotAnimal(a database.Animal) *animalRequest {
	var attrs animalAttributes
	_ = json.Unmarshal(a.Attributes, &attrs)
	return &animalRequest{Name: a.Name, Species: a.Species, Breed: a.Breed, Size: a.Size, Sex: a.Sex, Description: a.Description, HealthNotes: a.HealthNotes, BehaviorNotes: a.BehaviorNotes, Attributes: &attrs, Location: locationInput{City: a.City, State: a.StateCode, CountryCode: a.CountryCode}}
}
func (s *Server) createSurrender(w http.ResponseWriter, r *http.Request) {
	var req surrenderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	org, _ := pathUUIDValue(req.OrganizationID)
	if !s.publicOrganization(r, org) {
		writeServiceError(w, ErrConflict)
		return
	}
	if !s.checkMedia(w, r, req.Documents, "surrender") {
		return
	}
	var animalID, appID pgtype.UUID
	if req.Kind == "intake" && req.ApplicationID != nil {
		writeServiceError(w, ErrInvalid)
		return
	}
	if req.Kind == "return" {
		if req.ApplicationID == nil {
			writeServiceError(w, ErrInvalid)
			return
		}
		appID, _ = pathUUIDValue(*req.ApplicationID)
		app, err := s.queries.GetAdoptionApplicationByID(r.Context(), appID)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if app.AdopterID != actor || app.OrganizationID != org {
			writeServiceError(w, ErrForbidden)
			return
		}
		if app.Status != "completed" {
			writeServiceError(w, ErrConflict)
			return
		}
		animalID = app.AnimalID
		if req.AnimalID != nil && *req.AnimalID != uuidString(animalID) {
			writeServiceError(w, ErrInvalid)
			return
		}
		a, err := s.queries.GetAnimalByID(r.Context(), animalID)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if a.Status != "adopted" {
			writeServiceError(w, ErrConflict)
			return
		}
		var latest bool
		err = s.pool.QueryRow(r.Context(), "SELECT NOT EXISTS(SELECT 1 FROM adoption_applications WHERE animal_id=$1 AND status='completed' AND created_at>$2)", animalID, app.CreatedAt).Scan(&latest)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if !latest {
			writeServiceError(w, ErrConflict)
			return
		}
		if req.Animal == nil {
			req.Animal = snapshotAnimal(a)
		}
	} else if req.AnimalID != nil {
		animalID, _ = pathUUIDValue(*req.AnimalID)
		a, err := s.queries.GetAnimalByID(r.Context(), animalID)
		if err != nil {
			handleDBError(w, err)
			return
		}
		if a.SubmittedBy != actor || a.Source != "private_submission" {
			writeServiceError(w, ErrForbidden)
			return
		}
		if a.Status == "adopted" || a.Status == "archived" {
			writeServiceError(w, ErrConflict)
			return
		}
		if a.ResponsibleOrganizationID.Valid && a.ResponsibleOrganizationID != org {
			writeServiceError(w, ErrConflict)
			return
		}
		if req.Animal == nil {
			req.Animal = snapshotAnimal(a)
		}
	}
	if req.Animal == nil {
		writeServiceError(w, ErrInvalid)
		return
	}
	if errs := validateAnimal(req.Animal); !errs.empty() {
		writeProblem(w, 422, "Validation failed", "review the animal fields", errs)
		return
	}
	errs := fieldErrors{}
	birthDate(req.Animal.ApproximateBirthDate, errs)
	if !errs.empty() {
		writeProblem(w, 422, "Validation failed", "invalid animal birth date", errs)
		return
	}
	req.Animal.OrganizationID = &req.OrganizationID
	var id string
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	if animalID.Valid {
		if _, err = tx.Exec(r.Context(), "SELECT id FROM animals WHERE id=$1 FOR UPDATE", animalID); err != nil {
			handleDBError(w, err)
			return
		}
		var active bool
		if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM adoption_applications WHERE animal_id=$1 AND status IN ('submitted','screening','more_information_requested','approved'))", animalID).Scan(&active); err != nil {
			handleDBError(w, err)
			return
		}
		if active {
			writeServiceError(w, ErrConflict)
			return
		}
	}
	err = tx.QueryRow(r.Context(), "INSERT INTO surrenders(requester_id,organization_id,kind,animal_id,application_id,animal_snapshot,reason,documents) VALUES($1,$2,$3,$4,$5,$6,$7,COALESCE($8::uuid[],'{}')) RETURNING id::text", actor, org, req.Kind, animalID, appID, optionalJSON(req.Animal), req.Reason, req.Documents).Scan(&id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO surrender_history(surrender_id,actor_id,action,notes,snapshot) VALUES($1,$2,'submitted',$3,$4)", id, actor, req.Reason, optionalJSON(req.Animal)); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	r.SetPathValue("surrenderId", id)
	v, ok := s.surrenderScope(w, r)
	if ok {
		writeJSON(w, 201, v)
	}
}
func (s *Server) listSurrenders(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	var org pgtype.UUID
	if raw := r.URL.Query().Get("organization_id"); raw != "" {
		var ok bool
		org, ok = pathUUIDValue(raw)
		if !ok {
			invalidPath(w)
			return
		}
		if !s.analyticsAllowed(r, org) {
			writeServiceError(w, ErrForbidden)
			return
		}
	}
	offset, size, page := pagination(r)
	rows, err := s.pool.Query(r.Context(), "SELECT "+surrenderProjection+" FROM surrenders s WHERE CASE WHEN $2::uuid IS NULL THEN s.requester_id=$1 ELSE s.organization_id=$2 END ORDER BY s.created_at DESC,s.id LIMIT $3 OFFSET $4", actor, org, size, offset)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []surrenderResponse{}
	for rows.Next() {
		var v surrenderResponse
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
	page.Total = len(items)
	writeJSON(w, 200, collectionResponse[surrenderResponse]{Items: items, Page: page})
}
func (s *Server) resubmitSurrender(w http.ResponseWriter, r *http.Request) {
	v, ok := s.surrenderScope(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	if uuidString(actor) != v.RequesterID {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req surrenderResubmission
	if !decodeJSON(w, r, &req) || !s.checkMedia(w, r, req.Documents, "surrender") {
		return
	}
	if req.Animal == nil {
		req.Animal = &v.Animal
	}
	if errs := validateAnimal(req.Animal); !errs.empty() {
		writeProblem(w, 422, "Validation failed", "review animal fields", errs)
		return
	}
	errs := fieldErrors{}
	birthDate(req.Animal.ApproximateBirthDate, errs)
	if !errs.empty() {
		writeProblem(w, 422, "Validation failed", "invalid animal birth date", errs)
		return
	}
	req.Animal.OrganizationID = &v.OrganizationID
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	tag, err := tx.Exec(r.Context(), "UPDATE surrenders SET animal_snapshot=$2,reason=$3,documents=COALESCE($4::uuid[],'{}'),status='submitted',updated_at=now() WHERE id=$1 AND status='needs_information'", v.ID, optionalJSON(req.Animal), req.Reason, req.Documents)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO surrender_history(surrender_id,actor_id,action,notes,snapshot) VALUES($1,$2,'resubmitted',$3,$4)", v.ID, actor, req.Reason, optionalJSON(req.Animal)); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getSurrender(w, r)
}
func (s *Server) decideSurrender(w http.ResponseWriter, r *http.Request) {
	v, ok := s.surrenderScope(w, r)
	if !ok {
		return
	}
	org, _ := pathUUIDValue(v.OrganizationID)
	if !s.analyticsAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req surrenderDecision
	if !decodeJSON(w, r, &req) {
		return
	}
	s.changeSurrender(w, r, v, req.Decision, req.Reason)
}
func (s *Server) cancelSurrender(w http.ResponseWriter, r *http.Request) {
	v, ok := s.surrenderScope(w, r)
	if !ok {
		return
	}
	var req lifecycleReason
	if !decodeJSON(w, r, &req) {
		return
	}
	s.changeSurrender(w, r, v, "cancelled", req.Reason)
}
func (s *Server) changeSurrender(w http.ResponseWriter, r *http.Request, v surrenderResponse, status, reason string) {
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	tag, err := tx.Exec(r.Context(), `UPDATE surrenders SET status=$2,review_reason=$3,reviewed_by=$4,updated_at=now() WHERE id=$1 AND delivered_at IS NULL AND received_at IS NULL AND (status IN ('submitted','needs_information') OR (status='accepted' AND $2='cancelled'))`, v.ID, status, reason, actor)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO surrender_history(surrender_id,actor_id,action,notes) VALUES($1,$2,$3,$4)", v.ID, actor, status, reason); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getSurrender(w, r)
}
func (s *Server) signedSurrender(r *http.Request, id string) bool {
	var signed bool
	err := s.pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM agreements a WHERE a.surrender_id=$1 AND (SELECT count(*) FROM agreement_signatures WHERE agreement_id=a.id)=2)", id).Scan(&signed)
	return err == nil && signed
}
func (s *Server) sendSurrenderCode(w http.ResponseWriter, r *http.Request) {
	v, ok := s.surrenderScope(w, r)
	if !ok {
		return
	}
	actor, _ := subjectUUID(r)
	if uuidString(actor) != v.RequesterID {
		writeServiceError(w, ErrForbidden)
		return
	}
	if v.Status != "accepted" || v.ReceivedAt != nil || !s.signedSurrender(r, v.ID) {
		writeServiceError(w, ErrConflict)
		return
	}
	if s.email == nil {
		writeProblem(w, 503, "Email unavailable", "email delivery is not configured", nil)
		return
	}
	u, err := s.queries.GetUserByID(r.Context(), actor)
	if err != nil {
		handleDBError(w, err)
		return
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
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	tag, err := tx.Exec(r.Context(), "UPDATE surrenders SET handover_code_hash=$2,code_expires_at=now()+interval '10 minutes',code_failed_attempts=0 WHERE id=$1 AND status='accepted' AND (code_expires_at IS NULL OR code_expires_at<=now()+interval '9 minutes')", v.ID, hash)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeProblem(w, 429, "Try again later", "wait one minute before requesting another handover code", nil)
		return
	}
	if err = s.email.Send(r.Context(), u.Email, "PetFinder surrender handover", "Handover code for "+v.ID+": "+code); err != nil {
		writeProblem(w, 503, "Email unavailable", "unable to deliver handover code", nil)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(202)
}
func (s *Server) confirmSurrenderHandover(w http.ResponseWriter, r *http.Request) {
	v, ok := s.surrenderScope(w, r)
	if !ok {
		return
	}
	var req surrenderHandover
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	org, _ := pathUUIDValue(v.OrganizationID)
	if req.Type == "delivery" && uuidString(actor) != v.RequesterID {
		writeServiceError(w, ErrForbidden)
		return
	}
	if req.Type == "receipt" && !s.analyticsAllowed(r, org) {
		writeServiceError(w, ErrForbidden)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	var status string
	var hash *string
	var expires *time.Time
	var attempts int
	var delivered, received *time.Time
	var animalID pgtype.UUID
	err = tx.QueryRow(r.Context(), "SELECT status,handover_code_hash,code_expires_at,code_failed_attempts,delivered_at,received_at,animal_id FROM surrenders WHERE id=$1 FOR UPDATE", v.ID).Scan(&status, &hash, &expires, &attempts, &delivered, &received, &animalID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if status != "accepted" || !s.signedSurrender(r, v.ID) {
		writeServiceError(w, ErrConflict)
		return
	}
	if req.Type == "receipt" {
		if received != nil {
			writeServiceError(w, ErrConflict)
			return
		}
		if hash == nil || expires == nil || !expires.After(time.Now()) || attempts >= 5 {
			writeServiceError(w, ErrInvalid)
			return
		}
		if auth.VerifyPassword(*hash, req.Code) != nil {
			if _, err = tx.Exec(r.Context(), "UPDATE surrenders SET code_failed_attempts=code_failed_attempts+1 WHERE id=$1", v.ID); err != nil {
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
		if _, err = tx.Exec(r.Context(), "UPDATE surrenders SET received_at=now(),handover_code_hash=NULL WHERE id=$1", v.ID); err != nil {
			handleDBError(w, err)
			return
		}
		now := time.Now()
		received = &now
	} else {
		if delivered != nil {
			writeServiceError(w, ErrConflict)
			return
		}
		if _, err = tx.Exec(r.Context(), "UPDATE surrenders SET delivered_at=now() WHERE id=$1", v.ID); err != nil {
			handleDBError(w, err)
			return
		}
		now := time.Now()
		delivered = &now
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO surrender_history(surrender_id,actor_id,action,notes) VALUES($1,$2,$3,'Handover confirmed')", v.ID, actor, req.Type); err != nil {
		handleDBError(w, err)
		return
	}
	if delivered != nil && received != nil {
		if !animalID.Valid {
			base := animalParams(v.Animal)
			base.SubmittedBy, _ = pathUUIDValue(v.RequesterID)
			base.ResponsibleOrganizationID = org
			errs := fieldErrors{}
			base.BirthDate = birthDate(v.Animal.ApproximateBirthDate, errs)
			if !errs.empty() {
				writeServiceError(w, ErrInvalid)
				return
			}
			animal, err := database.New(tx).CreatePrivateAnimal(r.Context(), base)
			if err != nil {
				handleDBError(w, err)
				return
			}
			animalID = animal.ID
		}
		var before string
		var currentOrg pgtype.UUID
		if err = tx.QueryRow(r.Context(), "SELECT status,responsible_organization_id FROM animals WHERE id=$1 AND deleted_at IS NULL FOR UPDATE", animalID).Scan(&before, &currentOrg); err != nil {
			handleDBError(w, err)
			return
		}
		var active bool
		if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM adoption_applications WHERE animal_id=$1 AND status IN ('submitted','screening','more_information_requested','approved'))", animalID).Scan(&active); err != nil {
			handleDBError(w, err)
			return
		}
		if active || (v.Kind == "return" && before != "adopted") {
			writeServiceError(w, ErrConflict)
			return
		}
		if _, err = tx.Exec(r.Context(), "UPDATE animals SET responsible_organization_id=$2,status='available',organization_review_status='accepted',reviewed_by=$3,reviewed_at=now(),published_at=now() WHERE id=$1", animalID, org, actor); err != nil {
			handleDBError(w, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO animal_history(animal_id,actor_id,action,reason,previous_organization_id,organization_id,from_status,to_status) VALUES($1,$2,'surrender_handover',$3,$4,$5,$6,'available')`, animalID, actor, v.Reason, currentOrg, org, before); err != nil {
			handleDBError(w, err)
			return
		}
		if _, err = tx.Exec(r.Context(), "UPDATE surrenders SET status='completed',animal_id=$2,completed_at=now(),updated_at=now() WHERE id=$1", v.ID, animalID); err != nil {
			handleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getSurrender(w, r)
}
