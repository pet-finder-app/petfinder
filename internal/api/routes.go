package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

// registerRoutes is the single source of truth for routing and OpenAPI.
func registerRoutes(s *Server, mux *http.ServeMux) *huma.OpenAPI {
	reg := newRouteRegistry(mux)
	registerRoute[noBody, healthResponse](reg, http.MethodGet, "/health", "healthCheck", "Check database readiness", "System", 200, healthHandler(s.pool), serviceUnavailable)
	registerRoute[registerRequest, authSessionResponse](reg, http.MethodPost, "/v1/auth/register", "register", "Register an account", "Authentication", 201, http.HandlerFunc(s.register))
	registerRoute[loginRequest, authSessionResponse](reg, http.MethodPost, "/v1/auth/login", "login", "Sign in", "Authentication", 200, http.HandlerFunc(s.login))
	registerRoute[refreshRequest, authSessionResponse](reg, http.MethodPost, "/v1/auth/refresh", "refreshSession", "Rotate a refresh token", "Authentication", 200, http.HandlerFunc(s.refresh))
	registerRoute[refreshRequest, noBody](reg, http.MethodPost, "/v1/auth/logout", "logout", "Revoke a refresh session", "Authentication", 204, s.protected(s.logout), bearerAuth(false))
	registerRoute[noBody, userResponse](reg, http.MethodGet, "/v1/me", "getProfile", "Get your profile", "Profile", 200, s.protected(s.getProfile), bearerAuth(false))
	registerRoute[updateProfileRequest, userResponse](reg, http.MethodPatch, "/v1/me", "updateProfile", "Update your profile", "Profile", 200, s.protected(s.updateProfile), bearerAuth(false))
	registerRoute[noBody, collectionResponse[organizationResponse]](reg, http.MethodGet, "/v1/organizations", "listOrganizations", "List verified organizations", "Organizations", 200, http.HandlerFunc(s.listOrganizations), queryParameters[paginationQuery], queryParameters[organizationQuery])
	registerRoute[createOrganizationRequest, organizationResponse](reg, http.MethodPost, "/v1/organizations", "createOrganization", "Request organization registration", "Organizations", 201, s.protected(s.createOrganization), bearerAuth(false))
	registerRoute[noBody, organizationResponse](reg, http.MethodGet, "/v1/organizations/{organizationId}", "getOrganization", "Get an organization", "Organizations", 200, s.optionalAuth(s.getOrganization), bearerAuth(true))
	registerRoute[updateOrganizationRequest, organizationResponse](reg, http.MethodPatch, "/v1/organizations/{organizationId}", "updateOrganization", "Update an organization", "Organizations", 200, s.protected(s.updateOrganization), bearerAuth(false))
	registerRoute[organizationVerificationRequest, organizationResponse](reg, http.MethodPost, "/v1/organizations/{organizationId}/verification", "decideOrganizationVerification", "Verify or suspend an organization", "Organizations", 200, s.admin(s.verifyOrganization), adminOnly)
	registerRoute[noBody, memberListResponse](reg, http.MethodGet, "/v1/organizations/{organizationId}/members", "listOrganizationMembers", "List organization members", "Organizations", 200, s.protected(s.listOrganizationMembers), bearerAuth(false))
	registerRoute[addOrganizationMemberRequest, database.OrganizationMember](reg, http.MethodPost, "/v1/organizations/{organizationId}/members", "addOrganizationMember", "Invite an organization member", "Organizations", 201, s.protected(s.addOrganizationMember), bearerAuth(false))
	registerRoute[noBody, noBody](reg, http.MethodDelete, "/v1/organizations/{organizationId}/members/{userId}", "removeOrganizationMember", "Remove an organization member", "Organizations", 204, s.protected(s.removeOrganizationMember), bearerAuth(false))
	registerRoute[noBody, collectionResponse[database.ListAvailableAnimalsRow]](reg, http.MethodGet, "/v1/animals", "listAnimals", "Discover available animals", "Animals", 200, s.optionalAuth(s.listAnimals), bearerAuth(true), queryParameters[paginationQuery], queryParameters[animalQuery])
	registerRoute[animalRequest, database.Animal](reg, http.MethodPost, "/v1/animals", "createAnimal", "Submit or publish an animal", "Animals", 201, s.protected(s.createAnimal), bearerAuth(false))
	registerRoute[noBody, animalDetailResponse](reg, http.MethodGet, "/v1/animals/{animalId}", "getAnimal", "Get an animal and its gallery", "Animals", 200, s.optionalAuth(s.getAnimal), bearerAuth(true))
	registerRoute[updateAnimalRequest, database.Animal](reg, http.MethodPatch, "/v1/animals/{animalId}", "updateAnimal", "Update an animal", "Animals", 200, s.protected(s.updateAnimal), bearerAuth(false))
	registerRoute[animalStatusRequest, database.Animal](reg, http.MethodPatch, "/v1/animals/{animalId}/status", "updateAnimalStatus", "Change an animal lifecycle state", "Animals", 200, s.protected(s.updateAnimalStatus), bearerAuth(false))
	registerRoute[animalImageRequest, animalImageResponse](reg, http.MethodPost, "/v1/animals/{animalId}/images", "addAnimalImage", "Add an image for moderation", "Animals", 201, s.protected(s.addAnimalImage), bearerAuth(false))
	registerRoute[noBody, noBody](reg, http.MethodDelete, "/v1/animals/{animalId}/images/{imageId}", "removeAnimalImage", "Remove an animal image", "Animals", 204, s.protected(s.removeAnimalImage), bearerAuth(false))
	registerRoute[noBody, collectionResponse[database.ListPreferredAnimalsRow]](reg, http.MethodGet, "/v1/preferences", "listPreferences", "List liked or dismissed animals", "Preferences", 200, s.protected(s.listPreferences), bearerAuth(false), queryParameters[paginationQuery], queryParameters[preferenceQuery])
	registerRoute[preferenceRequest, database.AnimalPreference](reg, http.MethodPut, "/v1/animals/{animalId}/preference", "setAnimalPreference", "Like or dismiss an animal", "Preferences", 200, s.protected(s.setPreference), bearerAuth(false))
	registerRoute[noBody, collectionResponse[database.ListAdopterApplicationsRow]](reg, http.MethodGet, "/v1/applications", "listApplications", "List adoption applications", "Applications", 200, s.protected(s.listApplications), bearerAuth(false), queryParameters[paginationQuery], queryParameters[organizationIDQuery], queryParameters[applicationQuery], responseAlternative[collectionResponse[database.ListOrganizationApplicationsRow]])
	registerRoute[applicationRequest, applicationCreatedResponse](reg, http.MethodPost, "/v1/applications", "createApplication", "Request adoption of a liked animal", "Applications", 201, s.protected(s.createApplication), bearerAuth(false))
	registerRoute[noBody, applicationDetailResponse](reg, http.MethodGet, "/v1/applications/{applicationId}", "getApplication", "Get an application and screening history", "Applications", 200, s.protected(s.getApplication), bearerAuth(false))
	registerRoute[screeningRequest, database.ApplicationScreening](reg, http.MethodPut, "/v1/applications/{applicationId}/screening", "submitScreening", "Submit adoption screening answers", "Applications", 200, s.protected(s.submitScreening), bearerAuth(false))
	registerRoute[applicationDecisionRequest, database.AdoptionApplication](reg, http.MethodPost, "/v1/applications/{applicationId}/decision", "decideApplication", "Decide an adoption application", "Applications", 200, s.protected(s.decideApplication), bearerAuth(false))
	registerRoute[noBody, collectionResponse[database.ListUserConversationsRow]](reg, http.MethodGet, "/v1/conversations", "listConversations", "List organization-mediated conversations", "Conversations", 200, s.protected(s.listConversations), bearerAuth(false), queryParameters[paginationQuery], queryParameters[organizationIDQuery], responseAlternative[collectionResponse[database.ListOrganizationConversationsRow]])
	registerRoute[noBody, collectionResponse[database.Message]](reg, http.MethodGet, "/v1/conversations/{conversationId}/messages", "listMessages", "List messages in a mediated conversation", "Conversations", 200, s.protected(s.listMessages), bearerAuth(false), queryParameters[messagePaginationQuery])
	registerRoute[messageRequest, messageResponse](reg, http.MethodPost, "/v1/conversations/{conversationId}/messages", "sendMessage", "Send a message to the organization", "Conversations", 201, s.protected(s.sendMessage), bearerAuth(false))
	registerRoute[appointmentRequest, database.Appointment](reg, http.MethodPost, "/v1/applications/{applicationId}/appointments", "createAppointment", "Propose an adoption appointment", "Appointments", 201, s.protected(s.createAppointment), bearerAuth(false))
	registerRoute[updateAppointmentRequest, database.Appointment](reg, http.MethodPatch, "/v1/appointments/{appointmentId}", "updateAppointment", "Schedule or cancel an appointment", "Appointments", 200, s.protected(s.updateAppointment), bearerAuth(false))
	registerRoute[noBody, issuedOTPResponse](reg, http.MethodPost, "/v1/appointments/{appointmentId}/otp", "issueAppointmentOtp", "Issue a delivery OTP", "Appointments", 201, s.protected(s.issueAppointmentOTP), bearerAuth(false))
	registerRoute[appointmentConfirmationRequest, database.AppointmentConfirmation](reg, http.MethodPost, "/v1/appointments/{appointmentId}/confirmations", "confirmAppointment", "Confirm delivery or receipt", "Appointments", 200, s.protected(s.confirmAppointment), bearerAuth(false))
	registerRoute[noBody, collectionResponse[database.ListOpenReportsRow]](reg, http.MethodGet, "/v1/reports", "listReports", "List open moderation reports", "Reports", 200, s.protected(s.listReports), bearerAuth(false), queryParameters[paginationQuery], queryParameters[organizationIDQuery])
	registerRoute[reportRequest, database.Report](reg, http.MethodPost, "/v1/reports", "createReport", "Report suspicious adoption activity", "Reports", 201, s.protected(s.createReport), bearerAuth(false))
	registerRoute[noBody, database.Report](reg, http.MethodGet, "/v1/reports/{reportId}", "getReport", "Get a moderation report", "Reports", 200, s.protected(s.getReport), bearerAuth(false))
	registerRoute[updateReportRequest, database.Report](reg, http.MethodPatch, "/v1/reports/{reportId}", "updateReport", "Investigate or resolve a report", "Reports", 200, s.protected(s.updateReport), bearerAuth(false))
	return reg.spec
}
