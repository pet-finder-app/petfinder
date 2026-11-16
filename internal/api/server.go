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
	chat       *chatHub
	email      EmailSender
	resetURL   string
}

func NewServer(pool *pgxpool.Pool, tokens *auth.TokenManager, accessTTL, refreshTTL time.Duration) *Server {
	return &Server{pool: pool, queries: database.New(pool), tokens: tokens, accessTTL: accessTTL, refreshTTL: refreshTTL, chat: newChatHub()}
}

// CloseChat ends open chat WebSocket connections. http.Server.Shutdown does
// not track hijacked connections, so register it with RegisterOnShutdown.
func (s *Server) CloseChat() {
	s.chat.close()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	spec := registerRoutes(s, mux)
	serveOpenAPI(mux, spec)
	return commonMiddleware(mux)
}

func (s *Server) protected(handler http.HandlerFunc) http.Handler {
	return s.tokens.Authenticate(s.accountActive(handler))
}

func (s *Server) admin(handler http.HandlerFunc) http.Handler {
	return s.tokens.Authenticate(s.accountActive(auth.RequireRole(auth.RoleAdmin)(handler)))
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
		s.accountActive(handler).ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
	})
}

func (s *Server) accountActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.validAccount(r) {
			writeProblem(w, 401, "Unauthorized", "account or session is unavailable", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validAccount(r *http.Request) bool {
	id, ok := subjectUUID(r)
	if !ok {
		return false
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		return false
	}
	u, err := s.queries.GetUserByID(r.Context(), id)
	return err == nil && !u.DeactivatedAt.Valid && int(u.AuthVersion) == claims.Version
}
