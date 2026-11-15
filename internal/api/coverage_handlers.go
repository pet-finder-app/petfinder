package api

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
	"github.com/pet-finder-app/petfinder-api/internal/domain"
)

func (s *Server) listMyOrganizations(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	items, err := s.queries.ListMemberOrganizations(r.Context(), database.ListMemberOrganizationsParams{UserID: actor, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	views := make([]memberOrganizationResponse, 0, len(items))
	for _, item := range items {
		views = append(views, memberOrganizationResponse{Organization: organizationView(item.Organization, item.MembershipStatus == "active"), MembershipStatus: item.MembershipStatus, MemberRole: item.MemberRole})
	}
	page.Total = len(views)
	writeJSON(w, http.StatusOK, collectionResponse[memberOrganizationResponse]{Items: views, Page: page})
}

func (s *Server) listMyAnimals(w http.ResponseWriter, r *http.Request) {
	actor, _ := subjectUUID(r)
	offset, size, page := pagination(r)
	items, err := s.queries.ListAnimalsBySubmitter(r.Context(), database.ListAnimalsBySubmitterParams{SubmittedBy: actor, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.Animal]{Items: items, Page: page})
}

func (s *Server) listOrganizationAnimals(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	if _, allowed := s.activeMember(r, id, actor); !allowed && !hasRole(r, auth.RoleAdmin) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var status *string
	if value := strings.TrimSpace(r.URL.Query().Get("status")); value != "" {
		switch domain.AnimalStatus(value) {
		case domain.AnimalPending, domain.AnimalAvailable, domain.AnimalInAdoption, domain.AnimalAdopted, domain.AnimalUnavailable, domain.AnimalUnderReview:
			status = &value
		default:
			writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "invalid animal status", nil)
			return
		}
	}
	offset, size, page := pagination(r)
	items, err := s.queries.ListAnimalsByOrganization(r.Context(), database.ListAnimalsByOrganizationParams{OrganizationID: id, Status: status, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	page.Total = len(items)
	writeJSON(w, http.StatusOK, collectionResponse[database.Animal]{Items: items, Page: page})
}

func (s *Server) clearPreference(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "animalId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	// Idempotent and scoped to the actor, even if the animal was removed.
	if _, err := s.queries.DeleteAnimalPreference(r.Context(), database.DeleteAnimalPreferenceParams{UserID: actor, AnimalID: id}); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listApplicationAppointments(w http.ResponseWriter, r *http.Request) {
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
	items, err := s.queries.ListApplicationAppointments(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, appointmentListResponse{Items: items})
}

func (s *Server) getAppointment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "appointmentId")
	if !ok {
		invalidPath(w)
		return
	}
	appointment, err := s.queries.GetAppointmentByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	application, err := s.queries.GetAdoptionApplicationByID(r.Context(), appointment.ApplicationID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, application, false) {
		writeServiceError(w, ErrForbidden)
		return
	}
	confirmations, err := s.queries.ListAppointmentConfirmations(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, appointmentDetailResponse{Appointment: appointment, Confirmations: confirmations})
}

func (s *Server) withdrawApplication(w http.ResponseWriter, r *http.Request) {
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
	if application.AdopterID != actor {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request withdrawalRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	reason := strings.TrimSpace(request.Reason)
	if len(reason) < 5 || len(reason) > 2000 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "reason must contain between 5 and 2000 characters", nil)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	// Lock before reading the history's previous status. The UPDATE also checks
	// eligibility, so a concurrent decision cannot be overwritten by withdrawal.
	if _, err = database.New(tx).LockAdoptionApplication(r.Context(), id); err != nil {
		handleDBError(w, err)
		return
	}
	q := database.New(tx)
	application, err = q.GetAdoptionApplicationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	updated, err := q.WithdrawApplication(r.Context(), database.WithdrawApplicationParams{ID: id, AdopterID: actor})
	if err == pgx.ErrNoRows {
		writeServiceError(w, ErrConflict)
		return
	}
	if err != nil {
		handleDBError(w, err)
		return
	}
	from := application.Status
	if _, err = q.AddApplicationStatusHistory(r.Context(), database.AddApplicationStatusHistoryParams{ApplicationID: id, FromStatus: &from, ToStatus: string(domain.ApplicationWithdrawn), Reason: &reason, ChangedBy: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if err = q.CloseWithdrawnApplicationConversations(r.Context(), database.CloseWithdrawnApplicationConversationsParams{ApplicationID: id, ActorID: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
