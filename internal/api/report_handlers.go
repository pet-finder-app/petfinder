package api

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

var reportCategories = map[string]bool{"payment_request": true, "fraud": true, "harassment": true, "inappropriate_content": true, "off_platform_contact": true, "animal_welfare": true, "other": true}

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	var request struct {
		Category    string `json:"category"`
		Description string `json:"description"`
		SubjectType string `json:"subject_type"`
		SubjectID   string `json:"subject_id"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	subjectID, ok := pathUUIDValue(request.SubjectID)
	if !ok {
		invalidPath(w)
		return
	}
	request.Description = strings.TrimSpace(request.Description)
	if !reportCategories[request.Category] || len(request.Description) < 10 || len(request.Description) > 5000 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "invalid category or description", nil)
		return
	}
	params := database.CreateReportParams{ReporterID: actor, Category: request.Category, Description: request.Description, Priority: "normal"}
	switch request.SubjectType {
	case "organization":
		params.OrganizationID = subjectID
	case "animal":
		params.AnimalID = subjectID
	case "application":
		params.ApplicationID = subjectID
	case "conversation":
		params.ConversationID = subjectID
	case "message":
		params.MessageID = subjectID
	default:
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "invalid subject_type", nil)
		return
	}
	report, err := s.queries.CreateReport(r.Context(), params)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, report)
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	var orgID pgtype.UUID
	if value := strings.TrimSpace(r.URL.Query().Get("organization_id")); value != "" {
		var ok bool
		orgID, ok = pathUUIDValue(value)
		if !ok {
			invalidPath(w)
			return
		}
		if _, allowed := s.activeMember(r, orgID, actor); !allowed && !hasRole(r, auth.RoleAdmin) {
			writeServiceError(w, ErrForbidden)
			return
		}
	} else if !hasRole(r, auth.RoleAdmin) {
		writeServiceError(w, ErrForbidden)
		return
	}
	items, err := s.queries.ListOpenReports(r.Context(), database.ListOpenReportsParams{OrganizationID: orgID, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "page": page})
}

func (s *Server) reportAllowed(r *http.Request, report database.Report) bool {
	actor, ok := subjectUUID(r)
	if !ok {
		return false
	}
	if actor == report.ReporterID || hasRole(r, auth.RoleAdmin) {
		return true
	}
	if report.OrganizationID.Valid {
		_, allowed := s.activeMember(r, report.OrganizationID, actor)
		return allowed
	}
	return false
}

func (s *Server) getReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "reportId")
	if !ok {
		invalidPath(w)
		return
	}
	report, err := s.queries.GetReportByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.reportAllowed(r, report) {
		writeServiceError(w, ErrForbidden)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) updateReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "reportId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	report, err := s.queries.GetReportByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	allowed := hasRole(r, auth.RoleAdmin)
	if report.OrganizationID.Valid {
		_, member := s.activeMember(r, report.OrganizationID, actor)
		allowed = allowed || member
	}
	if !allowed {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Resolution = strings.TrimSpace(request.Resolution)
	if request.Status != "investigating" && request.Status != "resolved" && request.Status != "dismissed" {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "invalid report status", nil)
		return
	}
	var updated database.Report
	if request.Status == "investigating" {
		updated, err = s.queries.SetReportStatus(r.Context(), database.SetReportStatusParams{Status: request.Status, ID: id})
	} else {
		if len(request.Resolution) < 5 {
			writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "resolution must contain at least 5 characters", nil)
			return
		}
		updated, err = s.queries.ResolveReport(r.Context(), database.ResolveReportParams{Status: request.Status, Resolution: &request.Resolution, ResolvedBy: actor, ID: id})
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
