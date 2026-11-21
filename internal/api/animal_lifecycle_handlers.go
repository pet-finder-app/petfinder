package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
	"github.com/pet-finder-app/petfinder-api/internal/domain"
)

type animalAttributes struct {
	HeightCM          *float64 `json:"height_cm,omitempty" exclusiveMinimum:"0" maximum:"1000"`
	WeightKG          *float64 `json:"weight_kg,omitempty" exclusiveMinimum:"0" maximum:"1000"`
	Temperament       []string `json:"temperament,omitempty" maxItems:"30"`
	Vaccinated        *bool    `json:"vaccinated,omitempty"`
	Neutered          *bool    `json:"neutered,omitempty"`
	SpecialNeeds      bool     `json:"special_needs,omitempty"`
	MedicalConditions []string `json:"medical_conditions,omitempty" maxItems:"30"`
	EnergyLevel       string   `json:"energy_level,omitempty" enum:"low,medium,high"`
}
type animalAssignment struct {
	OrganizationID string `json:"organization_id" format:"uuid"`
	Reason         string `json:"reason" minLength:"5" maxLength:"1000"`
}
type lifecycleReason struct {
	Reason string `json:"reason" minLength:"5" maxLength:"1000"`
}
type animalHistoryRecord struct {
	PreviousOrganizationID *string   `json:"previous_organization_id"`
	OrganizationID         *string   `json:"organization_id"`
	ID                     int64     `json:"id"`
	Action                 string    `json:"action"`
	Reason                 string    `json:"reason"`
	FromStatus             string    `json:"from_status"`
	ToStatus               string    `json:"to_status"`
	ActorID                string    `json:"actor_id"`
	CreatedAt              time.Time `json:"created_at"`
}
type animalHistoryResponse struct {
	Items []animalHistoryRecord `json:"items"`
}

func validSpecies(s string) bool {
	switch s {
	case "dog", "cat", "bird", "rabbit", "reptile", "small_mammal", "other":
		return true
	}
	return false
}
func registerAnimalLifecycleRoutes(s *Server, reg *routeRegistry) {
	registerRoute[animalAssignment, database.Animal](reg, "PUT", "/v1/animals/{animalId}/organization", "assignAnimalOrganization", "Assign a private submission to a verified organization", "Animals", 200, s.protected(s.assignAnimalOrganization), bearerAuth(false))
	registerRoute[lifecycleReason, database.Animal](reg, "POST", "/v1/animals/{animalId}/archive", "archiveAnimal", "Archive an animal without active applications", "Animals", 200, s.protected(s.archiveAnimal), bearerAuth(false))
	registerRoute[lifecycleReason, noBody](reg, "DELETE", "/v1/animals/{animalId}", "deleteAnimal", "Retain and hide an animal without active applications", "Animals", 204, s.protected(s.deleteAnimal), bearerAuth(false))
	registerRoute[noBody, animalHistoryResponse](reg, "GET", "/v1/animals/{animalId}/history", "animalHistory", "Read animal lifecycle history", "Animals", 200, s.protected(s.animalHistory), bearerAuth(false))
	registerRoute[lifecycleReason, database.AdoptionApplication](reg, "POST", "/v1/applications/{applicationId}/cancellation", "cancelApplication", "Cancel an active application and related appointments", "Applications", 200, s.protected(s.cancelApplication), bearerAuth(false))
}
func (s *Server) assignAnimalOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	var req animalAssignment
	if !decodeJSON(w, r, &req) {
		return
	}
	orgID, _ := pathUUIDValue(req.OrganizationID)
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	if _, err = tx.Exec(r.Context(), "SELECT id FROM animals WHERE id=$1 FOR UPDATE", id); err != nil {
		handleDBError(w, err)
		return
	}
	animal, err := q.GetAnimalByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if actor != animal.SubmittedBy && !hasRole(r, auth.RoleAdmin) {
		writeServiceError(w, ErrForbidden)
		return
	}
	org, err := q.GetOrganizationByID(r.Context(), orgID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if org.VerificationStatus != "verified" || org.DeactivatedAt.Valid {
		writeServiceError(w, ErrConflict)
		return
	}
	// Reassignment is restricted to pending submissions before any mediated case exists.
	var related bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM adoption_applications WHERE animal_id=$1 UNION ALL SELECT 1 FROM conversations WHERE animal_id=$1)", id).Scan(&related); err != nil {
		handleDBError(w, err)
		return
	}
	if related {
		writeServiceError(w, ErrConflict)
		return
	}
	updated, err := q.AssignAnimalOrganization(r.Context(), database.AssignAnimalOrganizationParams{ID: id, OrganizationID: orgID})
	if err != nil {
		writeServiceError(w, ErrConflict)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO animal_history(animal_id,actor_id,action,reason,previous_organization_id,organization_id,from_status,to_status) VALUES($1,$2,'organization_assignment',$3,$4,$5,$6,$6)`, id, actor, req.Reason, animal.ResponsibleOrganizationID, orgID, animal.Status); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, updated)
}
func (s *Server) archiveAnimal(w http.ResponseWriter, r *http.Request) { s.retainAnimal(w, r, false) }
func (s *Server) deleteAnimal(w http.ResponseWriter, r *http.Request)  { s.retainAnimal(w, r, true) }
func (s *Server) retainAnimal(w http.ResponseWriter, r *http.Request, remove bool) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	var req lifecycleReason
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	if _, err = tx.Exec(r.Context(), "SELECT id FROM animals WHERE id=$1 FOR UPDATE", id); err != nil {
		handleDBError(w, err)
		return
	}
	animal, err := q.GetAnimalByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	allowed := submitterControls(animal, actor) || s.animalWriteAllowed(r, animal, true)
	if !allowed {
		writeServiceError(w, ErrForbidden)
		return
	}
	var active bool
	err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM adoption_applications WHERE animal_id=$1 AND status IN ('submitted','screening','more_information_requested','approved'))", id).Scan(&active)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if active {
		writeServiceError(w, ErrConflict)
		return
	}
	action := "archive"
	if remove {
		action = "delete"
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO animal_history(animal_id,actor_id,action,reason,previous_organization_id,organization_id,from_status,to_status) VALUES($1,$2,$3,$4,$5,$5,$6,'archived')`, id, actor, action, req.Reason, animal.ResponsibleOrganizationID, animal.Status); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE animals SET status='archived',deleted_at=CASE WHEN $2 THEN now() ELSE deleted_at END WHERE id=$1`, id, remove); err != nil {
		handleDBError(w, err)
		return
	}
	updated, err := q.GetAnimalByID(r.Context(), id)
	if err != nil && !remove {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	if remove {
		w.WriteHeader(204)
		return
	}
	writeJSON(w, 200, updated)
}
func (s *Server) animalHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var owner, org pgtype.UUID
	if err := s.pool.QueryRow(r.Context(), "SELECT submitted_by,responsible_organization_id FROM animals WHERE id=$1", id).Scan(&owner, &org); err != nil {
		handleDBError(w, err)
		return
	}
	_, member := s.activeMember(r, org, actor)
	if actor != owner && !member && !hasRole(r, auth.RoleAdmin) {
		writeServiceError(w, ErrForbidden)
		return
	}
	rows, err := s.pool.Query(r.Context(), "SELECT id,action,reason,from_status,to_status,actor_id::text,created_at,previous_organization_id::text,organization_id::text FROM animal_history WHERE animal_id=$1 ORDER BY id", id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []animalHistoryRecord{}
	for rows.Next() {
		var v animalHistoryRecord
		if err = rows.Scan(&v.ID, &v.Action, &v.Reason, &v.FromStatus, &v.ToStatus, &v.ActorID, &v.CreatedAt, &v.PreviousOrganizationID, &v.OrganizationID); err != nil {
			handleDBError(w, err)
			return
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, animalHistoryResponse{Items: items})
}
func (s *Server) cancelApplication(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	var req lifecycleReason
	if !decodeJSON(w, r, &req) {
		return
	}
	actor, _ := subjectUUID(r)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	if _, err = q.LockAdoptionApplication(r.Context(), id); err != nil {
		handleDBError(w, err)
		return
	}
	app, err := q.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, app, true) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if err = domain.ValidateApplicationTransition(domain.ApplicationStatus(app.Status), domain.ApplicationCanceled); err != nil || app.Status == "cancelled" {
		writeServiceError(w, ErrConflict)
		return
	}
	updated, err := q.SetApplicationStatus(r.Context(), database.SetApplicationStatusParams{ID: id, OrganizationID: app.OrganizationID, Status: "cancelled", Reason: &req.Reason, ActorID: actor})
	if err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.AddApplicationStatusHistory(r.Context(), database.AddApplicationStatusHistoryParams{ApplicationID: id, FromStatus: &app.Status, ToStatus: "cancelled", Reason: &req.Reason, ChangedBy: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if err = q.CloseWithdrawnApplicationConversations(r.Context(), database.CloseWithdrawnApplicationConversationsParams{ApplicationID: id, ActorID: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE appointments SET status='cancelled',cancelled_at=now(),cancelled_by=$2,cancellation_reason=$3 WHERE application_id=$1 AND status IN ('proposed','scheduled')", id, actor, req.Reason); err != nil {
		handleDBError(w, err)
		return
	}
	if app.Status == "approved" {
		if _, err = tx.Exec(r.Context(), `UPDATE animals SET status='available' WHERE id=$1 AND status='adoption_in_progress' AND NOT EXISTS(SELECT 1 FROM adoption_applications WHERE animal_id=$1 AND status='approved')`, app.AnimalID); err != nil {
			handleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, updated)
}
