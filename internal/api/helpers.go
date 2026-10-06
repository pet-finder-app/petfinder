package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

func pagination(r *http.Request) (int32, int32, pageResponse) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return int32((page - 1) * size), int32(size), pageResponse{Page: page, PageSize: size}
}

func pathUUID(r *http.Request, name string) (pgtype.UUID, bool) {
	parsed, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func subjectUUID(r *http.Request) (pgtype.UUID, bool) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		return pgtype.UUID{}, false
	}
	parsed, err := uuid.Parse(subject)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, true
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func timestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	utc := value.Time.UTC()
	return &utc
}

func numeric(value *float64) pgtype.Numeric {
	if value == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*value, 'f', 6, 64))
	return n
}

func userView(user database.User, roles []database.Role) userResponse {
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Key)
	}
	var location *locationResponse
	if user.City != nil || user.StateCode != nil {
		location = &locationResponse{City: user.City, State: user.StateCode, CountryCode: user.CountryCode}
	}
	return userResponse{
		ID: uuidString(user.ID), Name: user.DisplayName, Email: user.Email,
		Phone: user.Phone, Roles: roleNames, Location: location,
		CreatedAt: timestamp(user.CreatedAt), UpdatedAt: timestamp(user.UpdatedAt),
	}
}

func organizationView(org database.Organization, privileged bool) organizationResponse {
	result := organizationResponse{
		ID: uuidString(org.ID), Name: org.DisplayName, Description: org.Description,
		VerificationStatus: org.VerificationStatus, VerifiedAt: timestamp(org.VerifiedAt),
		OfficialChannels: officialChannelsResponse{Website: org.WebsiteUrl, Email: org.OfficialEmail, Phone: org.OfficialPhone},
		Location:         locationResponse{City: &org.City, State: &org.StateCode, CountryCode: org.CountryCode},
		CreatedAt:        timestamp(org.CreatedAt), UpdatedAt: timestamp(org.UpdatedAt),
	}
	if privileged {
		result.LegalName = &org.LegalName
		result.RegistrationNumber = &org.RegistrationNumber
		result.VerificationNotes = &org.VerificationNotes
	}
	return result
}

func roleClaims(roles []database.Role) []auth.Role {
	result := make([]auth.Role, 0, len(roles))
	for _, role := range roles {
		result = append(result, auth.Role(role.Key))
	}
	return result
}

func randomToken(bytes int) (string, []byte, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(value)
	hash := sha256.Sum256([]byte(encoded))
	return encoded, hash[:], nil
}

func clientAddress(r *http.Request) *netip.Addr {
	address := r.RemoteAddr
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		address = forwarded
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	parsed, parseErr := netip.ParseAddr(strings.Trim(host, "[]"))
	if parseErr != nil {
		return nil
	}
	return &parsed
}

func (s *Server) createRefreshSession(r *http.Request, q *database.Queries, userID pgtype.UUID) (string, database.RefreshSession, error) {
	token, hash, err := randomToken(32)
	if err != nil {
		return "", database.RefreshSession{}, err
	}
	ua := strings.TrimSpace(r.UserAgent())
	var userAgent *string
	if ua != "" {
		userAgent = &ua
	}
	session, err := q.CreateRefreshSession(r.Context(), database.CreateRefreshSessionParams{
		UserID: userID, TokenHash: hash, UserAgent: userAgent, IpAddress: clientAddress(r),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(s.refreshTTL), Valid: true},
	})
	return token, session, err
}

func (s *Server) authResponse(user database.User, roles []database.Role, access, refresh string) authSessionResponse {
	return authSessionResponse{
		AccessToken: access, RefreshToken: refresh, TokenType: "Bearer",
		ExpiresIn: int(s.accessTTL.Seconds()), User: userView(user, roles),
	}
}

func invalidPath(w http.ResponseWriter) {
	writeProblem(w, http.StatusBadRequest, "Invalid path parameter", "an identifier must be a valid UUID", nil)
}

func handleDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeServiceError(w, ErrNotFound)
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			writeServiceError(w, ErrConflict)
		case "23503", "23514", "22P02":
			writeServiceError(w, ErrInvalid)
		default:
			writeServiceError(w, fmt.Errorf("database operation: %w", err))
		}
		return
	}
	writeServiceError(w, err)
}

func hasRole(r *http.Request, role auth.Role) bool {
	claims, ok := auth.ClaimsFromContext(r.Context())
	return ok && claims.HasRole(role)
}

func (s *Server) activeMember(r *http.Request, organizationID, userID pgtype.UUID) (database.OrganizationMember, bool) {
	member, err := s.queries.GetActiveOrganizationMember(r.Context(), database.GetActiveOrganizationMemberParams{
		OrganizationID: organizationID, UserID: userID,
	})
	return member, err == nil
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	dash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
