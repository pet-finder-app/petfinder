package api

import (
	"net/http"
	"strings"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

type officialChannels struct {
	Website *string `json:"website"`
	Email   string  `json:"email"`
	Phone   *string `json:"phone"`
}

type createOrganizationRequest struct {
	Name               string           `json:"name"`
	LegalName          string           `json:"legal_name"`
	RegistrationNumber string           `json:"registration_number"`
	Description        string           `json:"description"`
	OfficialChannels   officialChannels `json:"official_channels"`
	Location           locationInput    `json:"location"`
}

func validateOrganization(request *createOrganizationRequest) fieldErrors {
	errs := fieldErrors{}
	request.Name = required(errs, "name", request.Name, 120)
	request.LegalName = required(errs, "legal_name", request.LegalName, 180)
	request.RegistrationNumber = required(errs, "registration_number", request.RegistrationNumber, 32)
	if len(request.RegistrationNumber) < 8 {
		errs.add("registration_number", "must contain at least 8 characters")
	}
	request.Description = required(errs, "description", request.Description, 2000)
	if len(request.Description) < 20 {
		errs.add("description", "must contain at least 20 characters")
	}
	request.OfficialChannels.Email = validEmail(errs, request.OfficialChannels.Email)
	request.Location.City = required(errs, "location.city", request.Location.City, 100)
	request.Location.State = required(errs, "location.state", request.Location.State, 100)
	request.Location.CountryCode = strings.ToUpper(strings.TrimSpace(request.Location.CountryCode))
	if len(request.Location.CountryCode) != 2 {
		errs.add("location.country_code", "must be an ISO 3166-1 alpha-2 code")
	}
	return errs
}

func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request) {
	offset, size, page := pagination(r)
	var state, city *string
	if value := strings.TrimSpace(r.URL.Query().Get("state")); value != "" {
		state = &value
	}
	if value := strings.TrimSpace(r.URL.Query().Get("city")); value != "" {
		city = &value
	}
	items, err := s.queries.ListVerifiedOrganizations(r.Context(), database.ListVerifiedOrganizationsParams{StateCode: state, City: city, PageOffset: offset, PageSize: size})
	if err != nil {
		handleDBError(w, err)
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, item := range items {
		views = append(views, organizationView(item, false))
	}
	page.Total = len(views)
	writeJSON(w, http.StatusOK, map[string]any{"items": views, "page": page})
}

func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request) {
	actor, ok := subjectUUID(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid identity", nil)
		return
	}
	var request createOrganizationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if errs := validateOrganization(&request); !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := database.New(tx)
	registration := request.RegistrationNumber
	org, err := q.CreateOrganization(r.Context(), database.CreateOrganizationParams{
		LegalName: request.LegalName, DisplayName: request.Name, Slug: slugify(request.Name), RegistrationNumber: &registration,
		Description: request.Description, OfficialEmail: request.OfficialChannels.Email, OfficialPhone: request.OfficialChannels.Phone,
		WebsiteUrl: request.OfficialChannels.Website, City: request.Location.City, StateCode: request.Location.State,
		CountryCode: request.Location.CountryCode, Latitude: numeric(request.Location.Latitude), Longitude: numeric(request.Location.Longitude), CreatedBy: actor,
	})
	if err != nil {
		handleDBError(w, err)
		return
	}
	member, err := q.InviteOrganizationMember(r.Context(), database.InviteOrganizationMemberParams{
		OrganizationID: org.ID, UserID: actor, MemberRole: "owner", CanManageMembers: true,
		CanPublishAnimals: true, CanManageApplications: true, InvitedBy: actor,
	})
	if err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.AcceptOrganizationMembership(r.Context(), database.AcceptOrganizationMembershipParams{ID: member.ID, UserID: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if err = q.GrantUserRole(r.Context(), database.GrantUserRoleParams{UserID: actor, GrantedBy: actor, RoleKey: string(auth.RoleNGOMember)}); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, organizationView(org, true))
}

func (s *Server) getOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	org, err := s.queries.GetOrganizationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	privileged := false
	if actor, valid := subjectUUID(r); valid {
		_, privileged = s.activeMember(r, id, actor)
		privileged = privileged || hasRole(r, auth.RoleAdmin)
	}
	if org.VerificationStatus != "verified" && !privileged {
		writeServiceError(w, ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, organizationView(org, privileged))
}

func (s *Server) updateOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, ok := subjectUUID(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid identity", nil)
		return
	}
	member, memberOK := s.activeMember(r, id, actor)
	if !hasRole(r, auth.RoleAdmin) && (!memberOK || !member.CanManageMembers) {
		writeServiceError(w, ErrForbidden)
		return
	}
	current, err := s.queries.GetOrganizationByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	var request struct {
		Name             *string           `json:"name"`
		Description      *string           `json:"description"`
		OfficialChannels *officialChannels `json:"official_channels"`
		Location         *locationInput    `json:"location"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	name, description, email, phone, website := current.DisplayName, current.Description, current.OfficialEmail, current.OfficialPhone, current.WebsiteUrl
	city, state, country, lat, lng := current.City, current.StateCode, current.CountryCode, current.Latitude, current.Longitude
	errs := fieldErrors{}
	if request.Name != nil {
		name = required(errs, "name", *request.Name, 120)
	}
	if request.Description != nil {
		description = required(errs, "description", *request.Description, 2000)
	}
	if request.OfficialChannels != nil {
		if request.OfficialChannels.Email != "" {
			email = validEmail(errs, request.OfficialChannels.Email)
		}
		phone, website = request.OfficialChannels.Phone, request.OfficialChannels.Website
	}
	if request.Location != nil {
		city = required(errs, "location.city", request.Location.City, 100)
		state = required(errs, "location.state", request.Location.State, 100)
		country = strings.ToUpper(request.Location.CountryCode)
		lat, lng = numeric(request.Location.Latitude), numeric(request.Location.Longitude)
	}
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	org, err := s.queries.UpdateOrganizationProfile(r.Context(), database.UpdateOrganizationProfileParams{DisplayName: name, Description: description, OfficialEmail: email, OfficialPhone: phone, WebsiteUrl: website, City: city, StateCode: state, CountryCode: country, Latitude: lat, Longitude: lng, ID: id})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, organizationView(org, true))
}

func (s *Server) verifyOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var request struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	valid := request.Decision == "verified" || request.Decision == "rejected" || request.Decision == "suspended"
	if !valid || len(strings.TrimSpace(request.Reason)) < 10 {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "decision and a reason of at least 10 characters are required", nil)
		return
	}
	reason := strings.TrimSpace(request.Reason)
	org, err := s.queries.SetOrganizationVerification(r.Context(), database.SetOrganizationVerificationParams{VerificationStatus: request.Decision, VerificationNotes: &reason, ActorID: actor, ID: id})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, organizationView(org, true))
}

func (s *Server) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
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
	members, err := s.queries.ListOrganizationMembers(r.Context(), database.ListOrganizationMembersParams{OrganizationID: id})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": members})
}

func (s *Server) addOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	member, allowed := s.activeMember(r, orgID, actor)
	if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageMembers) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if request.Role != "coordinator" && request.Role != "representative" && request.Role != "moderator" {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "invalid member role", nil)
		return
	}
	user, err := s.queries.GetActiveUserByEmail(r.Context(), request.Email)
	if err != nil {
		handleDBError(w, err)
		return
	}
	canManage := request.Role == "coordinator"
	invited, err := s.queries.InviteOrganizationMember(r.Context(), database.InviteOrganizationMemberParams{OrganizationID: orgID, UserID: user.ID, MemberRole: request.Role, CanManageMembers: canManage, CanPublishAnimals: request.Role != "moderator", CanManageApplications: true, InvitedBy: actor})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invited)
}

func (s *Server) removeOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := pathUUID(r, "organizationId")
	if !ok {
		invalidPath(w)
		return
	}
	userID, ok := pathUUID(r, "userId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	member, allowed := s.activeMember(r, orgID, actor)
	if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageMembers) {
		writeServiceError(w, ErrForbidden)
		return
	}
	members, err := s.queries.ListOrganizationMembers(r.Context(), database.ListOrganizationMembersParams{OrganizationID: orgID})
	if err != nil {
		handleDBError(w, err)
		return
	}
	for _, candidate := range members {
		if candidate.UserID == userID {
			if candidate.MemberRole == "owner" {
				writeServiceError(w, ErrInvalid)
				return
			}
			affected, err := s.queries.RemoveOrganizationMember(r.Context(), database.RemoveOrganizationMemberParams{ID: candidate.ID, OrganizationID: orgID})
			if err != nil {
				handleDBError(w, err)
				return
			}
			if affected == 0 {
				writeServiceError(w, ErrNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeServiceError(w, ErrNotFound)
}
