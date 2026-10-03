package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

type Server struct {
	pool       *pgxpool.Pool
	queries    *database.Queries
	tokens     *auth.TokenManager
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewServer(pool *pgxpool.Pool, tokens *auth.TokenManager, accessTTL, refreshTTL time.Duration) *Server {
	return &Server{pool: pool, queries: database.New(pool), tokens: tokens, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", healthHandler(s.pool))
	mux.HandleFunc("POST /v1/auth/register", s.register)
	mux.HandleFunc("POST /v1/auth/login", s.login)
	mux.HandleFunc("POST /v1/auth/refresh", s.refresh)
	mux.Handle("POST /v1/auth/logout", s.protected(s.logout))

	mux.Handle("GET /v1/me", s.protected(s.getProfile))
	mux.Handle("PATCH /v1/me", s.protected(s.updateProfile))

	mux.HandleFunc("GET /v1/organizations", s.listOrganizations)
	mux.Handle("POST /v1/organizations", s.protected(s.createOrganization))
	mux.Handle("GET /v1/organizations/{organizationId}", s.optionalAuth(s.getOrganization))
	mux.Handle("PATCH /v1/organizations/{organizationId}", s.protected(s.updateOrganization))
	mux.Handle("POST /v1/organizations/{organizationId}/verification", s.admin(s.verifyOrganization))
	mux.Handle("GET /v1/organizations/{organizationId}/members", s.protected(s.listOrganizationMembers))
	mux.Handle("POST /v1/organizations/{organizationId}/members", s.protected(s.addOrganizationMember))
	mux.Handle("DELETE /v1/organizations/{organizationId}/members/{userId}", s.protected(s.removeOrganizationMember))

	mux.Handle("GET /v1/animals", s.optionalAuth(s.listAnimals))
	mux.Handle("POST /v1/animals", s.protected(s.createAnimal))
	mux.Handle("GET /v1/animals/{animalId}", s.optionalAuth(s.getAnimal))
	mux.Handle("PATCH /v1/animals/{animalId}", s.protected(s.updateAnimal))
	mux.Handle("PATCH /v1/animals/{animalId}/status", s.protected(s.updateAnimalStatus))
	mux.Handle("POST /v1/animals/{animalId}/images", s.protected(s.addAnimalImage))
	mux.Handle("DELETE /v1/animals/{animalId}/images/{imageId}", s.protected(s.removeAnimalImage))
	mux.Handle("GET /v1/preferences", s.protected(s.listPreferences))
	mux.Handle("PUT /v1/animals/{animalId}/preference", s.protected(s.setPreference))

	mux.Handle("GET /v1/applications", s.protected(s.listApplications))
	mux.Handle("POST /v1/applications", s.protected(s.createApplication))
	mux.Handle("GET /v1/applications/{applicationId}", s.protected(s.getApplication))
	mux.Handle("PUT /v1/applications/{applicationId}/screening", s.protected(s.submitScreening))
	mux.Handle("POST /v1/applications/{applicationId}/decision", s.protected(s.decideApplication))

	mux.Handle("GET /v1/conversations", s.protected(s.listConversations))
	mux.Handle("GET /v1/conversations/{conversationId}/messages", s.protected(s.listMessages))
	mux.Handle("POST /v1/conversations/{conversationId}/messages", s.protected(s.sendMessage))

	mux.Handle("POST /v1/applications/{applicationId}/appointments", s.protected(s.createAppointment))
	mux.Handle("PATCH /v1/appointments/{appointmentId}", s.protected(s.updateAppointment))
	mux.Handle("POST /v1/appointments/{appointmentId}/otp", s.protected(s.issueAppointmentOTP))
	mux.Handle("POST /v1/appointments/{appointmentId}/confirmations", s.protected(s.confirmAppointment))

	mux.Handle("GET /v1/reports", s.protected(s.listReports))
	mux.Handle("POST /v1/reports", s.protected(s.createReport))
	mux.Handle("GET /v1/reports/{reportId}", s.protected(s.getReport))
	mux.Handle("PATCH /v1/reports/{reportId}", s.protected(s.updateReport))

	return commonMiddleware(mux)
}

func (s *Server) protected(handler http.HandlerFunc) http.Handler {
	return s.tokens.Authenticate(handler)
}

func (s *Server) admin(handler http.HandlerFunc) http.Handler {
	return s.tokens.Authenticate(auth.RequireRole(auth.RoleAdmin)(handler))
}

func (s *Server) optionalAuth(handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" {
			handler.ServeHTTP(w, r)
			return
		}
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid bearer token", nil)
			return
		}
		claims, err := s.tokens.Parse(parts[1])
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid bearer token", nil)
			return
		}
		handler.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
	})
}
