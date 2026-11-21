package api

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

type EmailSender interface {
	Send(context.Context, string, string, string) error
}

// ConfigureEmail enables real email delivery for recovery and agreement signing.
func (s *Server) ConfigureEmail(sender EmailSender, resetURL string) {
	s.email, s.resetURL = sender, resetURL
}

type forgotPasswordRequest struct {
	Email string `json:"email" format:"email" maxLength:"254"`
}
type resetPasswordRequest struct {
	Token    string `json:"token" minLength:"32" maxLength:"200" writeOnly:"true"`
	Password string `json:"password" minLength:"12" maxLength:"72" writeOnly:"true"`
}
type deleteAccountRequest struct {
	Password string `json:"password" minLength:"1" maxLength:"72" writeOnly:"true"`
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var request forgotPasswordRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if s.email == nil || s.resetURL == "" {
		writeProblem(w, 503, "Email unavailable", "email delivery is not configured", nil)
		return
	}
	errs := fieldErrors{}
	email := validEmail(errs, request.Email)
	if !errs.empty() {
		writeServiceError(w, ErrInvalid)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	u, err := q.GetActiveUserByEmail(r.Context(), email)
	if err != nil {
		w.WriteHeader(202)
		return
	}
	// Serialize requests for this account; never expose the recovery token over HTTP.
	if _, err = tx.Exec(r.Context(), "SELECT id FROM users WHERE id=$1 FOR UPDATE", u.ID); err != nil {
		handleDBError(w, err)
		return
	}
	var recent bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM password_reset_tokens WHERE user_id=$1 AND created_at > now()-interval '1 minute')", u.ID).Scan(&recent); err != nil {
		handleDBError(w, err)
		return
	}
	if recent {
		w.WriteHeader(202)
		return
	}
	token, hash, err := randomToken(32)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.RevokeUserPasswordResetTokens(r.Context(), u.ID); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.CreatePasswordResetToken(r.Context(), database.CreatePasswordResetTokenParams{UserID: u.ID, TokenHash: hash, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(30 * time.Minute), Valid: true}}); err != nil {
		handleDBError(w, err)
		return
	}
	link, err := url.Parse(s.resetURL)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	query := link.Query()
	query.Set("token", token)
	link.RawQuery = query.Encode()
	if err = s.email.Send(r.Context(), u.Email, "Petfinder: redefinição de senha", "Recebemos um pedido para redefinir a senha da sua conta.\n\nAbra este link em até 30 minutos para escolher uma nova senha:\n"+link.String()+"\n\nSe você não fez este pedido, ignore este e-mail; sua senha continua a mesma."); err != nil {
		writeProblem(w, 503, "Email unavailable", "unable to deliver recovery email", nil)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(202)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var request resetPasswordRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	errs := fieldErrors{}
	validPassword(errs, request.Password)
	if !errs.empty() {
		writeServiceError(w, ErrInvalid)
		return
	}
	hash := sha256.Sum256([]byte(request.Token))
	password, err := auth.HashPassword(request.Password)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	token, err := q.GetActivePasswordResetTokenByHash(r.Context(), hash[:])
	if err != nil {
		writeServiceError(w, ErrInvalid)
		return
	}
	n, err := q.ConsumePasswordResetToken(r.Context(), token.ID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if n != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	if n, err = q.UpdateUserPassword(r.Context(), database.UpdateUserPasswordParams{ID: token.UserID, PasswordHash: password}); err != nil || n != 1 {
		writeServiceError(w, ErrConflict)
		return
	}
	reason := "password_reset"
	if _, err = q.RevokeAllUserRefreshSessions(r.Context(), database.RevokeAllUserRefreshSessionsParams{UserID: token.UserID, RevokeReason: &reason}); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.RevokeUserPasswordResetTokens(r.Context(), token.UserID); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	var request deleteAccountRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	id, _ := subjectUUID(r)
	u, err := s.queries.GetUserByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if err = auth.VerifyPassword(u.PasswordHash, request.Password); err != nil {
		writeServiceError(w, ErrForbidden)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		handleDBError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	// Keep adoption, welfare and financial records; never cascade-delete them.
	var soleOwner bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organization_members m WHERE m.user_id=$1 AND m.status='active' AND m.member_role='owner' AND NOT EXISTS(SELECT 1 FROM organization_members n WHERE n.organization_id=m.organization_id AND n.user_id<>$1 AND n.status='active' AND n.can_manage_members))`, id).Scan(&soleOwner)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if soleOwner {
		writeProblem(w, 409, "Organization ownership", "transfer organization management before deactivation", nil)
		return
	}
	if _, err = q.DeactivateUser(r.Context(), id); err != nil {
		handleDBError(w, err)
		return
	}
	reason := "account_deactivated"
	if _, err = q.RevokeAllUserRefreshSessions(r.Context(), database.RevokeAllUserRefreshSessionsParams{UserID: id, RevokeReason: &reason}); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.RevokeUserPasswordResetTokens(r.Context(), id); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE organization_members SET status='removed',deactivated_at=now() WHERE user_id=$1 AND status IN ('active','invited')", id); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		handleDBError(w, err)
		return
	}
	w.WriteHeader(204)
}
