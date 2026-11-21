package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

type followupScheduleRequest struct {
	DueAt        time.Time `json:"due_at"`
	Instructions string    `json:"instructions" minLength:"5" maxLength:"5000"`
}
type followupReportRequest struct {
	Notes         string   `json:"notes" minLength:"10" maxLength:"10000"`
	WelfareStatus string   `json:"welfare_status" enum:"good,needs_attention,urgent"`
	MediaIDs      []string `json:"media_ids,omitempty" maxItems:"20"`
}
type followupReviewRequest struct {
	Decision string `json:"decision" enum:"closed,changes_requested"`
	Reason   string `json:"reason" minLength:"5" maxLength:"3000"`
}
type followupReport struct {
	ID            string    `json:"id"`
	SubmittedBy   string    `json:"submitted_by"`
	Notes         string    `json:"notes"`
	WelfareStatus string    `json:"welfare_status"`
	MediaIDs      []string  `json:"media_ids"`
	SubmittedAt   time.Time `json:"submitted_at"`
}
type followupReview struct {
	Decision   string    `json:"decision"`
	Reason     string    `json:"reason"`
	ReviewedBy string    `json:"reviewed_by"`
	ReviewedAt time.Time `json:"reviewed_at"`
}
type followupResponse struct {
	ID            string           `json:"id"`
	ApplicationID string           `json:"application_id"`
	DueAt         time.Time        `json:"due_at"`
	Instructions  string           `json:"instructions"`
	Status        string           `json:"status"`
	ClosedAt      *time.Time       `json:"closed_at"`
	Reports       []followupReport `json:"reports"`
	Reviews       []followupReview `json:"reviews"`
}
type followupList struct {
	Items []followupResponse `json:"items"`
}

const followupProjection = `jsonb_build_object('id',f.id,'application_id',f.application_id,'due_at',f.due_at,'instructions',f.instructions,'status',f.status,'closed_at',f.closed_at,'reports',COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.submitted_at,r.id) FROM followup_reports r WHERE r.followup_id=f.id),'[]'::jsonb),'reviews',COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY v.reviewed_at,v.id) FROM followup_reviews v WHERE v.followup_id=f.id),'[]'::jsonb))`

func registerFollowupRoutes(s *Server, reg *routeRegistry) {
	registerRoute[followupScheduleRequest, followupResponse](reg, "POST", "/v1/applications/{applicationId}/follow-ups", "scheduleFollowup", "Schedule post-adoption reporting", "Follow-ups", 201, s.protected(s.scheduleFollowup), bearerAuth(false))
	registerRoute[noBody, followupList](reg, "GET", "/v1/applications/{applicationId}/follow-ups", "listFollowups", "Read post-adoption reports and reviews", "Follow-ups", 200, s.protected(s.listFollowups), bearerAuth(false))
	registerRoute[noBody, followupResponse](reg, "GET", "/v1/follow-ups/{followupId}", "getFollowup", "Read a follow-up report task", "Follow-ups", 200, s.protected(s.getFollowup), bearerAuth(false))
	registerRoute[followupReportRequest, followupResponse](reg, "POST", "/v1/follow-ups/{followupId}/reports", "submitFollowupReport", "Submit an immutable follow-up report", "Follow-ups", 200, s.protected(s.submitFollowupReport), bearerAuth(false))
	registerRoute[followupReviewRequest, followupResponse](reg, "POST", "/v1/follow-ups/{followupId}/review", "reviewFollowupReport", "Request changes or close a submitted follow-up", "Follow-ups", 200, s.protected(s.reviewFollowup), bearerAuth(false))
}
func (s *Server) scheduleFollowup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	app, err := s.queries.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, app, true) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if app.Status != "completed" {
		writeServiceError(w, ErrConflict)
		return
	}
	var req followupScheduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !req.DueAt.After(time.Now()) {
		writeServiceError(w, ErrInvalid)
		return
	}
	actor, _ := subjectUUID(r)
	var task string
	err = s.pool.QueryRow(r.Context(), "INSERT INTO followup_tasks(application_id,organization_id,due_at,instructions,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id::text", id, app.OrganizationID, req.DueAt, req.Instructions, actor).Scan(&task)
	if err != nil {
		handleDBError(w, err)
		return
	}
	var v followupResponse
	err = s.pool.QueryRow(r.Context(), "SELECT "+followupProjection+" FROM followup_tasks f WHERE id=$1", task).Scan(&v)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 201, v)
}
func (s *Server) listFollowups(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	app, err := s.queries.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, app, false) {
		writeServiceError(w, ErrForbidden)
		return
	}
	rows, err := s.pool.Query(r.Context(), "SELECT "+followupProjection+" FROM followup_tasks f WHERE application_id=$1 ORDER BY due_at,id", id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rows.Close()
	items := []followupResponse{}
	for rows.Next() {
		var v followupResponse
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
	writeJSON(w, 200, followupList{Items: items})
}
func (s *Server) followupScope(w http.ResponseWriter, r *http.Request) (pgtype.UUID, database.GetAdoptionApplicationByIDRow, bool) {
	id, ok := pathUUID(r, "followupId")
	if !ok {
		invalidPath(w)
		return id, database.GetAdoptionApplicationByIDRow{}, false
	}
	var appID pgtype.UUID
	if err := s.pool.QueryRow(r.Context(), "SELECT application_id FROM followup_tasks WHERE id=$1", id).Scan(&appID); err != nil {
		handleDBError(w, err)
		return id, database.GetAdoptionApplicationByIDRow{}, false
	}
	app, err := s.queries.GetAdoptionApplicationByID(r.Context(), appID)
	if err != nil {
		handleDBError(w, err)
		return id, app, false
	}
	if !s.applicationAccess(r, app, false) {
		writeServiceError(w, ErrForbidden)
		return id, app, false
	}
	return id, app, true
}
func (s *Server) getFollowup(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.followupScope(w, r)
	if !ok {
		return
	}
	var v followupResponse
	if err := s.pool.QueryRow(r.Context(), "SELECT "+followupProjection+" FROM followup_tasks f WHERE id=$1", id).Scan(&v); err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) submitFollowupReport(w http.ResponseWriter, r *http.Request) {
	id, app, ok := s.followupScope(w, r)
	if !ok {
		return
	}
	// The organization reviews reports; letting it also author them would let
	// it close welfare follow-ups without the adopter.
	actor, _ := subjectUUID(r)
	if actor != app.AdopterID {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req followupReportRequest
	if !decodeJSON(w, r, &req) || !s.checkMedia(w, r, req.MediaIDs, "followup") {
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	tag, err := tx.Exec(r.Context(), "UPDATE followup_tasks SET status='submitted' WHERE id=$1 AND status IN ('scheduled','changes_requested')", id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO followup_reports(followup_id,submitted_by,notes,welfare_status,media_ids) VALUES($1,$2,$3,$4,COALESCE($5::uuid[],'{}'))", id, actor, req.Notes, req.WelfareStatus, req.MediaIDs); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getFollowup(w, r)
}
func (s *Server) reviewFollowup(w http.ResponseWriter, r *http.Request) {
	id, app, ok := s.followupScope(w, r)
	if !ok {
		return
	}
	if !s.applicationAccess(r, app, true) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var req followupReviewRequest
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
	tag, err := tx.Exec(r.Context(), "UPDATE followup_tasks SET status=$2,closed_at=CASE WHEN $2='closed' THEN now() ELSE NULL END WHERE id=$1 AND status='submitted'", id, req.Decision)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO followup_reviews(followup_id,reviewed_by,decision,reason) VALUES($1,$2,$3,$4)", id, actor, req.Decision, req.Reason); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	s.getFollowup(w, r)
}
func (s *Server) materializeDueFollowups(r *http.Request) error {
	actor, _ := subjectUUID(r)
	_, err := s.pool.Exec(r.Context(), `INSERT INTO notifications(user_id,kind,subject_id) SELECT $1,'followup_due',f.id FROM followup_tasks f JOIN adoption_applications a ON a.id=f.application_id WHERE a.adopter_id=$1 AND f.due_at<=now() AND f.status IN ('scheduled','changes_requested') ON CONFLICT(user_id,subject_id) WHERE kind='followup_due' DO NOTHING`, actor)
	return err
}
