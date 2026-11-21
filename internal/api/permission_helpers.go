package api

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"net/http"
)

func (s *Server) financeAllowed(r *http.Request, org pgtype.UUID) bool {
	actor, _ := subjectUUID(r)
	member, ok := s.activeMember(r, org, actor)
	return hasRole(r, auth.RoleAdmin) || (ok && member.CanManageMembers)
}
