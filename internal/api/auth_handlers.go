package api

import (
	"crypto/sha256"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	errs := fieldErrors{}
	request.Name = required(errs, "name", request.Name, 120)
	request.Email = validEmail(errs, request.Email)
	validPassword(errs, request.Password)
	if !request.AcceptedTerms {
		errs.add("accepted_terms", "must be accepted")
	}
	if !request.AcceptedPrivacyPolicy {
		errs.add("accepted_privacy_policy", "must be accepted")
	}
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}

	passwordHash, err := auth.HashPassword(request.Password)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := database.New(tx)
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	user, err := q.CreateUser(r.Context(), database.CreateUserParams{
		Email: request.Email, PasswordHash: passwordHash, DisplayName: request.Name,
		CountryCode: "BR", TermsAcceptedAt: now, PrivacyAcceptedAt: now,
	})
	if err != nil {
		handleDBError(w, err)
		return
	}
	for _, role := range []string{string(auth.RoleAdopter), string(auth.RoleAdvertiser)} {
		if err := q.GrantUserRole(r.Context(), database.GrantUserRoleParams{UserID: user.ID, RoleKey: role}); err != nil {
			handleDBError(w, err)
			return
		}
	}
	roles, err := q.ListUserRoles(r.Context(), user.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	refresh, _, err := s.createRefreshSession(r, q, user.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	access, err := s.tokens.Issue(uuidString(user.ID), roleClaims(roles))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.authResponse(user, roles, access, refresh))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	user, err := s.queries.GetActiveUserByEmail(r.Context(), request.Email)
	if err != nil || auth.VerifyPassword(user.PasswordHash, request.Password) != nil {
		writeProblem(w, http.StatusUnauthorized, "Invalid credentials", "email or password is invalid", nil)
		return
	}
	roles, err := s.queries.ListUserRoles(r.Context(), user.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	refresh, _, err := s.createRefreshSession(r, s.queries, user.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	access, err := s.tokens.Issue(uuidString(user.ID), roleClaims(roles))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.authResponse(user, roles, access, refresh))
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var request refreshRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if len(request.RefreshToken) < 32 {
		writeProblem(w, http.StatusUnauthorized, "Invalid session", "refresh token is invalid", nil)
		return
	}
	hash := sha256.Sum256([]byte(request.RefreshToken))
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	q := database.New(tx)
	session, err := q.GetActiveRefreshSessionByTokenHash(r.Context(), hash[:])
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "Invalid session", "refresh token is invalid or expired", nil)
		return
	}
	user, err := q.GetUserByID(r.Context(), session.UserID)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "Invalid session", "account is unavailable", nil)
		return
	}
	roles, err := q.ListUserRoles(r.Context(), user.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	newToken, replacement, err := s.createRefreshSession(r, q, user.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	affected, err := q.RotateRefreshSession(r.Context(), database.RotateRefreshSessionParams{ReplacedByID: replacement.ID, ID: session.ID})
	if err != nil || affected != 1 {
		writeProblem(w, http.StatusUnauthorized, "Invalid session", "refresh token was already used", nil)
		return
	}
	access, err := s.tokens.Issue(uuidString(user.ID), roleClaims(roles))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.authResponse(user, roles, access, newToken))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var request refreshRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	hash := sha256.Sum256([]byte(request.RefreshToken))
	session, err := s.queries.GetActiveRefreshSessionByTokenHash(r.Context(), hash[:])
	if err == nil {
		reason := "logout"
		_, err = s.queries.RevokeRefreshSession(r.Context(), database.RevokeRefreshSessionParams{RevokeReason: &reason, ID: session.ID})
	}
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := subjectUUID(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid identity", nil)
		return
	}
	user, err := s.queries.GetUserByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	roles, err := s.queries.ListUserRoles(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userView(user, roles))
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := subjectUUID(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "Unauthorized", "invalid identity", nil)
		return
	}
	var request updateProfileRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	current, err := s.queries.GetUserByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	name, phone, city, state, country := current.DisplayName, current.Phone, current.City, current.StateCode, current.CountryCode
	lat, lng := current.Latitude, current.Longitude
	errs := fieldErrors{}
	if request.Name != nil {
		name = required(errs, "name", *request.Name, 120)
	}
	if request.Phone != nil {
		v := optional(errs, "phone", *request.Phone, 30)
		phone = &v
	}
	if request.Location != nil {
		request.Location.City = required(errs, "location.city", request.Location.City, 100)
		request.Location.State = required(errs, "location.state", request.Location.State, 100)
		if len(request.Location.CountryCode) != 2 {
			errs.add("location.country_code", "must be an ISO 3166-1 alpha-2 code")
		}
		city, state, country = &request.Location.City, &request.Location.State, strings.ToUpper(request.Location.CountryCode)
		lat, lng = numeric(request.Location.Latitude), numeric(request.Location.Longitude)
	}
	if !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	user, err := s.queries.UpdateUserProfile(r.Context(), database.UpdateUserProfileParams{DisplayName: name, Phone: phone, City: city, StateCode: state, CountryCode: country, Latitude: lat, Longitude: lng, ID: id})
	if err != nil {
		handleDBError(w, err)
		return
	}
	roles, err := s.queries.ListUserRoles(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, userView(user, roles))
}
