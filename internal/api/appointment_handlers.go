package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/pet-finder-app/petfinder-api/internal/auth"
	"github.com/pet-finder-app/petfinder-api/internal/database"
	"github.com/pet-finder-app/petfinder-api/internal/domain"
)

func validateAppointment(request *appointmentRequest) fieldErrors {
	errs := fieldErrors{}
	switch request.Purpose {
	case "interview", "home_visit", "meet_and_greet", "delivery", "follow_up":
	default:
		errs.add("purpose", "is invalid")
	}
	futureTime(errs, "starts_at", request.StartsAt)
	if !request.EndsAt.After(request.StartsAt) {
		errs.add("ends_at", "must be after starts_at")
	}
	if _, err := time.LoadLocation(request.Timezone); err != nil {
		errs.add("timezone", "must be a valid IANA timezone")
	}
	request.LocationName = required(errs, "location_name", request.LocationName, 200)
	request.Address = required(errs, "address", request.Address, 500)
	request.Instructions = optional(errs, "instructions", request.Instructions, 2000)
	return errs
}

func (s *Server) createAppointment(w http.ResponseWriter, r *http.Request) {
	applicationID, ok := pathUUID(r, "applicationId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	application, err := s.queries.GetAdoptionApplicationByID(r.Context(), applicationID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	if !s.applicationAccess(r, application, true) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if application.Status != string(domain.ApplicationAccepted) {
		writeServiceError(w, ErrConflict)
		return
	}
	var request appointmentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if errs := validateAppointment(&request); !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	appointment, err := s.queries.CreateAppointment(r.Context(), database.CreateAppointmentParams{
		ApplicationID: applicationID, OrganizationID: application.OrganizationID, Kind: request.Purpose,
		StartsAt: pgtype.Timestamptz{Time: request.StartsAt.UTC(), Valid: true}, EndsAt: pgtype.Timestamptz{Time: request.EndsAt.UTC(), Valid: true},
		Timezone: request.Timezone, LocationName: request.LocationName, Address: request.Address, Instructions: request.Instructions,
		OtpRequired: request.Purpose == "delivery", CreatedBy: actor,
	})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, appointment)
}

func (s *Server) updateAppointment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "appointmentId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	current, err := s.queries.GetAppointmentByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	member, allowed := s.activeMember(r, current.OrganizationID, actor)
	if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageApplications) {
		writeServiceError(w, ErrForbidden)
		return
	}
	var request updateAppointmentRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Status != nil && *request.Status == "cancelled" {
		reason := "cancelled by organization"
		if request.Reason != nil {
			reason = strings.TrimSpace(*request.Reason)
		}
		if len(reason) < 5 {
			writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "reason must contain at least 5 characters", nil)
			return
		}
		updated, err := s.queries.CancelAppointment(r.Context(), database.CancelAppointmentParams{CancelledBy: actor, CancellationReason: &reason, ID: id})
		if err != nil {
			handleDBError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	starts, ends, timezone := current.StartsAt.Time, current.EndsAt.Time, current.Timezone
	locationName, address, instructions := current.LocationName, current.Address, current.Instructions
	if request.StartsAt != nil {
		starts = *request.StartsAt
	}
	if request.EndsAt != nil {
		ends = *request.EndsAt
	}
	if request.Timezone != nil {
		timezone = *request.Timezone
	}
	if request.LocationName != nil {
		locationName = *request.LocationName
	}
	if request.Address != nil {
		address = *request.Address
	}
	if request.Instructions != nil {
		instructions = *request.Instructions
	}
	validation := appointmentRequest{Purpose: current.Kind, StartsAt: starts, EndsAt: ends, Timezone: timezone, LocationName: locationName, Address: address, Instructions: instructions}
	if errs := validateAppointment(&validation); !errs.empty() {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", errs)
		return
	}
	updated, err := s.queries.ScheduleAppointment(r.Context(), database.ScheduleAppointmentParams{StartsAt: pgtype.Timestamptz{Time: starts.UTC(), Valid: true}, EndsAt: pgtype.Timestamptz{Time: ends.UTC(), Valid: true}, Timezone: timezone, LocationName: locationName, Address: address, Instructions: instructions, ID: id})
	if err != nil {
		handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func otpCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	value := n.String()
	return strings.Repeat("0", 6-len(value)) + value, nil
}

func (s *Server) issueAppointmentOTP(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "appointmentId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	appointment, err := s.queries.GetAppointmentByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	member, allowed := s.activeMember(r, appointment.OrganizationID, actor)
	if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageApplications) {
		writeServiceError(w, ErrForbidden)
		return
	}
	if appointment.Kind != "delivery" || appointment.Status == "cancelled" || appointment.Status == "completed" {
		writeServiceError(w, ErrConflict)
		return
	}
	code, err := otpCode()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	hash := sha256.Sum256([]byte(code))
	expires := time.Now().UTC().Add(15 * time.Minute)
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	if _, err = q.RevokeAppointmentOTP(r.Context(), id); err != nil {
		handleDBError(w, err)
		return
	}
	if _, err = q.CreateAppointmentOTP(r.Context(), database.CreateAppointmentOTPParams{AppointmentID: id, CodeHash: hash[:], ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}, MaxAttempts: 5, CreatedBy: actor}); err != nil {
		handleDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, issuedOTPResponse{Code: code, ExpiresAt: expires})
}

func (s *Server) confirmAppointment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(r, "appointmentId")
	if !ok {
		invalidPath(w)
		return
	}
	actor, _ := subjectUUID(r)
	var request appointmentConfirmationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Type != "delivery" && request.Type != "receipt" {
		writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "type must be delivery or receipt", nil)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer rollbackTransaction(r.Context(), tx)
	q := database.New(tx)
	appointment, err := q.GetAppointmentByID(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	application, err := q.GetAdoptionApplicationByID(r.Context(), appointment.ApplicationID)
	if err != nil {
		handleDBError(w, err)
		return
	}
	party, confirmation := "organization", "animal_delivered"
	if request.Type == "delivery" {
		member, allowed := s.activeMember(r, appointment.OrganizationID, actor)
		if !hasRole(r, auth.RoleAdmin) && (!allowed || !member.CanManageApplications) {
			writeServiceError(w, ErrForbidden)
			return
		}
	} else {
		party, confirmation = "adopter", "animal_received"
		if actor != application.AdopterID {
			writeServiceError(w, ErrForbidden)
			return
		}
		if len(request.OTPCode) != 6 {
			writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "otp_code must contain six digits", nil)
			return
		}
		otp, otpErr := q.GetActiveAppointmentOTP(r.Context(), id)
		if otpErr != nil {
			writeProblem(w, http.StatusUnauthorized, "Invalid OTP", "code is invalid or expired", nil)
			return
		}
		hash := sha256.Sum256([]byte(request.OTPCode))
		if subtle.ConstantTimeCompare(hash[:], otp.CodeHash) != 1 {
			_, _ = q.IncrementAppointmentOTPFailure(r.Context(), otp.ID)
			_ = tx.Commit(r.Context())
			writeProblem(w, http.StatusUnauthorized, "Invalid OTP", "code is invalid or expired", nil)
			return
		}
		affected, consumeErr := q.ConsumeAppointmentOTP(r.Context(), otp.ID)
		if consumeErr != nil || affected != 1 {
			writeProblem(w, http.StatusUnauthorized, "Invalid OTP", "code is invalid or expired", nil)
			return
		}
	}
	confirmed, err := q.UpsertAppointmentConfirmation(r.Context(), database.UpsertAppointmentConfirmationParams{AppointmentID: id, Party: party, ConfirmedBy: actor, Confirmation: confirmation})
	if err != nil {
		handleDBError(w, err)
		return
	}
	confirmations, err := q.ListAppointmentConfirmations(r.Context(), id)
	if err != nil {
		handleDBError(w, err)
		return
	}
	delivered, received := false, false
	for _, item := range confirmations {
		delivered = delivered || item.Confirmation == "animal_delivered"
		received = received || item.Confirmation == "animal_received"
	}
	if delivered && received {
		if _, err = q.CompleteAppointment(r.Context(), id); err != nil {
			handleDBError(w, err)
			return
		}
		reason := "delivery and receipt confirmed"
		if _, err = q.SetApplicationStatus(r.Context(), database.SetApplicationStatusParams{Status: string(domain.ApplicationComplete), Reason: &reason, ActorID: actor, ID: application.ID, OrganizationID: application.OrganizationID}); err != nil {
			handleDBError(w, err)
			return
		}
		if _, err = q.SetAnimalStatus(r.Context(), database.SetAnimalStatusParams{Status: string(domain.AnimalAdopted), Reason: &reason, ID: application.AnimalID, OrganizationID: application.OrganizationID}); err != nil {
			handleDBError(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, confirmed)
}
