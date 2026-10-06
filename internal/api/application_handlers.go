package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
	"github.com/pet-finder-app/petfinder-api/internal/domain"
	"github.com/pet-finder-app/petfinder-api/internal/moderation"
)

func (s *Server) listApplications(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	statusValue := strings.TrimSpace(r.URL.Query().Get("status"))
	var status *string
	if statusValue != "" {
		status = &statusValue
	}
	if rawOrg := strings.TrimSpace(r.URL.Query().Get("organization_id")); rawOrg != "" {
		orgID, ok := pathUUIDValue(rawOrg)
		if !ok {
			invalidPath(w)
			return
		}
		member, allowed := s.activeMember(r, orgID, actor)
		if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageApplications) {
			writeServiceError(w, ErrForbidden)
			return
		}
		items, err := s.queries.ListOrganizationApplications(r.Context(), database.ListOrganizationApplicationsParams{OrganizationID: orgID, Status: status, PageOffset: offset, PageSize: size})
		if err != nil {
			handleDBError(w, err)
			return
		}
		page.Total = len(items)
		writeJSON(w, http.StatusOK, collectionResponse[database.ListOrganizationApplicationsRow]{Items: items, Page: page})
		return
	}
	items, err := s.queries.ListAdopterApplications(r.Context(), database.ListAdopterApplicationsParams{AdopterID: actor, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.ListAdopterApplicationsRow]{Items: items, Page: page})
}

func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	var request applicationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	animalID, ok := pathUUIDValue(request.AnimalID)
	if !ok {
		invalidPath(w)
		return
	}
	request.Introduction = strings.TrimSpace(request.Introduction)
	if len(request.Introduction) < 20 || len(request.Introduction) > 3000 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "introduction must contain between 20 and 3000 characters", nil)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := database.New(tx)
	application, err := q.CreateAdoptionApplication(r.Context(), database.CreateAdoptionApplicationParams{AdopterID: actor, ApplicantMessage: request.Introduction, AnimalID: animalID})
	if err != nil {
		handleDBError(w, err)
		return
	}
	conversation, err := q.CreateAdopterConversation(r.Context(), application.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, applicationCreatedResponse{Application: application, ConversationID: uuidString(conversation.ID)})
}

func (s *Server) applicationAccess(r *http.Request, application database.GetAdoptionApplicationByIDRow, manage bool) bool {
	actor, ok := subjectUUID(r)
	if !ok {
		return false
	}
	if hasRole(r, auth.RoleAdmin) {
		return true
	}
	if !manage && actor == application.AdopterID {
		return true
	}
	member, allowed := s.activeMember(r, application.OrganizationID, actor)
	return allowed && (!manage || member.CanManageApplications)
}

func (s *Server) getApplication(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	application, err := s.queries.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, application, false) {
		writeServiceError(w, ErrForbidden)
		return
	}
	history, err := s.queries.ListApplicationStatusHistory(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	screening, screeningErr := s.queries.GetLatestApplicationScreening(r.Context(), id)
	response := applicationDetailResponse{Application: application, History: history}
	if screeningErr == nil {
		response.Screening = &screening
	} else if screeningErr != pgx.ErrNoRows {
		handleDBError(w, screeningErr)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) submitScreening(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	application, err := s.queries.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if actor != application.AdopterID {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request screeningRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	errs := fieldErrors{}
	request.Housing = required(errs, "housing", request.Housing, 1000)
	request.Household = required(errs, "household", request.Household, 1000)
	request.Experience = required(errs, "experience", request.Experience, 2000)
	request.CarePlan = required(errs, "care_plan", request.CarePlan, 3000)
	if len(request.CarePlan) < 20 {
		errs.add("care_plan", "must contain at least 20 characters")
	}
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	answers, _ := json.Marshal(request)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := database.New(tx)
	screening, err := q.GetLatestApplicationScreening(r.Context(), id)
	if err == pgx.ErrNoRows {
		questions := []byte(`["housing","household","experience","care_plan","has_other_animals","accepts_follow_up"]`)
		screening, err = q.CreateApplicationScreening(r.Context(), database.CreateApplicationScreeningParams{ApplicationID: id, Version: 1, Questions: questions, RequestedBy: actor})
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	screening, err = q.SubmitApplicationScreening(r.Context(), database.SubmitApplicationScreeningParams{Answers: answers, ID: screening.ID})
	if err != nil {
		handleDBError(w, err)
		return
	}
	if application.Status == string(domain.ApplicationPending) || application.Status == string(domain.ApplicationMoreInfo) {
		_, err = q.SetApplicationStatus(r.Context(), database.SetApplicationStatusParams{Status: string(domain.ApplicationScreening), ActorID: actor, ID: id, OrganizationID: application.OrganizationID})
		if err != nil {
			handleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, screening)
}

func (s *Server) decideApplication(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	application, err := s.queries.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, application, true) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request applicationDecisionRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	to := request.Decision
	if to == "needs_information" {
		to = string(domain.ApplicationMoreInfo)
	}
	if to != string(domain.ApplicationAccepted) && to != string(domain.ApplicationRejected) && to != string(domain.ApplicationMoreInfo) {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "invalid decision", nil)
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if len(reason) < 5 || len(reason) > 2000 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "reason must contain between 5 and 2000 characters", nil)
		return
	}
	if err := domain.ValidateApplicationTransition(domain.ApplicationStatus(application.Status), domain.ApplicationStatus(to)); err != nil {
		writeProblem(w, http.StatusConflict, "Invalid state transition", err.Error(), nil)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := database.New(tx)
	updated, err := q.SetApplicationStatus(r.Context(), database.SetApplicationStatusParams{Status: to, Reason: &reason, ActorID: actor, ID: id, OrganizationID: application.OrganizationID})
	if err != nil {
		handleDBError(w, err)
		return
	}
	from := application.Status
	if _, err = q.AddApplicationStatusHistory(r.Context(), database.AddApplicationStatusHistoryParams{ApplicationID: id, FromStatus: &from, ToStatus: to, Reason: &reason, ChangedBy: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if to == string(domain.ApplicationAccepted) {
		if _, err = q.SetAnimalStatus(r.Context(), database.SetAnimalStatusParams{Status: string(domain.AnimalInAdoption), Reason: &reason, ID: application.AnimalID, OrganizationID: application.OrganizationID}); err != nil {
			handleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	if rawOrg := strings.TrimSpace(r.URL.Query().Get("organization_id")); rawOrg != "" {
		orgID, ok := pathUUIDValue(rawOrg)
		if !ok {
			invalidPath(w)
			return
		}
		if _, allowed := s.activeMember(r, orgID, actor); !allowed && !hasRole(r, auth.RoleAdmin) {
			writeServiceError(w, ErrForbidden)
			return
		}
		items, err := s.queries.ListOrganizationConversations(r.Context(), database.ListOrganizationConversationsParams{OrganizationID: orgID, PageOffset: offset, PageSize: size})
		if err != nil {
			handleDBError(w, err)
			return
		}
		page.Total = len(items)
		writeJSON(w, http.StatusOK, collectionResponse[database.ListOrganizationConversationsRow]{Items: items, Page: page})
		return
	}
	items, err := s.queries.ListUserConversations(r.Context(), database.ListUserConversationsParams{UserID: actor, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.ListUserConversationsRow]{Items: items, Page: page})
}

func (s *Server) conversationAllowed(r *http.Request, conversation database.Conversation) (bool, bool) {
	actor, ok := subjectUUID(r)
	if !ok {
		return false, false
	}
	if actor == conversation.CounterpartyUserID {
		return true, false
	}
	_, member := s.activeMember(r, conversation.OrganizationID, actor)
	return member || hasRole(r, auth.RoleAdmin), member || hasRole(r, auth.RoleAdmin)
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "conversationId")
	if !ok {
		invalidPath(w)
		return
	}
	conversation, err := s.queries.GetConversationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	allowed, moderator := s.conversationAllowed(r, conversation)
	if !allowed {
		writeServiceError(w, ErrForbidden)
		return
	}
	_, size, page := pagination(r)
	items, err := s.queries.ListConversationMessages(r.Context(), database.ListConversationMessagesParams{ConversationID: id, IncludeModerated: moderator, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.Message]{Items: items, Page: page})
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "conversationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	conversation, err := s.queries.GetConversationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	allowed, moderator := s.conversationAllowed(r, conversation)
	if !allowed || conversation.Status != "open" {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request messageRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Body = strings.TrimSpace(request.Body)
	if len(request.Body) < 1 || len(request.Body) > 5000 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "body must contain between 1 and 5000 characters", nil)
		return
	}
	message, err := s.queries.CreateMessage(r.Context(), database.CreateMessageParams{ConversationID: id, SenderID: actor, Body: request.Body})
	if err != nil {
		handleDBError(w, err)
		return
	}
	inspection := moderation.Inspect(request.Body)
	if inspection.Flagged {
		reason := strings.Join(inspection.Reasons, ",")
		message, err = s.queries.ModerateMessage(r.Context(), database.ModerateMessageParams{ModerationStatus: "flagged", ModerationReason: &reason, ModeratedBy: actor, ID: message.ID})
		if err != nil {
			handleDBError(w, err)
			return
		}
	}
	response := messageResponse{Message: message}
	if inspection.Flagged && !moderator {
		response.Warning = "message flagged for organization review"
	}
	writeJSON(w, http.StatusCreated, response)
}

var _ = pgtype.UUID{}
