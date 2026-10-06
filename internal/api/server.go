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
	spec := registerRoutes(s, mux)
	serveOpenAPI(mux, spec)
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
