-- name: CreateAppointment :one
INSERT INTO appointments (
    application_id, organization_id, kind, starts_at, ends_at, timezone,
    location_name, address, latitude, longitude, instructions, otp_required, created_by
) VALUES (
    sqlc.arg(application_id), sqlc.arg(organization_id), sqlc.arg(kind),
    sqlc.arg(starts_at), sqlc.arg(ends_at), sqlc.arg(timezone),
    sqlc.arg(location_name), sqlc.arg(address), sqlc.narg(latitude),
    sqlc.narg(longitude), sqlc.arg(instructions), sqlc.arg(otp_required),
    sqlc.arg(created_by)
)
RETURNING *;

-- name: GetAppointmentByID :one
SELECT * FROM appointments WHERE id = sqlc.arg(id);

-- name: ListApplicationAppointments :many
SELECT * FROM appointments
WHERE application_id = sqlc.arg(application_id)
ORDER BY starts_at DESC, id;

-- name: ListUpcomingOrganizationAppointments :many
SELECT ap.*, a.name AS animal_name, u.display_name AS adopter_name
FROM appointments ap
JOIN adoption_applications aa ON aa.id = ap.application_id
JOIN animals a ON a.id = aa.animal_id
JOIN users u ON u.id = aa.adopter_id
WHERE ap.organization_id = sqlc.arg(organization_id)
  AND ap.status IN ('proposed', 'scheduled')
  AND ap.starts_at >= sqlc.arg(starts_after)
  AND ap.starts_at < sqlc.arg(starts_before)
ORDER BY ap.starts_at, ap.id;

-- name: ScheduleAppointment :one
UPDATE appointments
SET status = 'scheduled', starts_at = sqlc.arg(starts_at), ends_at = sqlc.arg(ends_at),
    timezone = sqlc.arg(timezone), location_name = sqlc.arg(location_name),
    address = sqlc.arg(address), latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude), instructions = sqlc.arg(instructions)
WHERE id = sqlc.arg(id) AND status IN ('proposed', 'scheduled')
RETURNING *;

-- name: CompleteAppointment :one
UPDATE appointments
SET status = 'completed'
WHERE id = sqlc.arg(id) AND status = 'scheduled'
RETURNING *;

-- name: CancelAppointment :one
UPDATE appointments
SET status = 'cancelled', cancelled_by = sqlc.arg(cancelled_by),
    cancelled_at = now(), cancellation_reason = sqlc.arg(cancellation_reason)
WHERE id = sqlc.arg(id) AND status IN ('proposed', 'scheduled')
RETURNING *;

-- name: UpsertAppointmentConfirmation :one
INSERT INTO appointment_confirmations (
    appointment_id, party, confirmed_by, confirmation, notes
) VALUES (
    sqlc.arg(appointment_id), sqlc.arg(party), sqlc.arg(confirmed_by),
    sqlc.arg(confirmation), sqlc.narg(notes)
)
ON CONFLICT (appointment_id, party, confirmation) DO UPDATE
SET confirmed_by = EXCLUDED.confirmed_by,
    confirmed_at = now(),
    notes = EXCLUDED.notes
RETURNING *;

-- name: ListAppointmentConfirmations :many
SELECT * FROM appointment_confirmations
WHERE appointment_id = sqlc.arg(appointment_id)
ORDER BY confirmed_at, id;

-- name: CreateAppointmentOTP :one
INSERT INTO appointment_otps (
    appointment_id, code_hash, expires_at, max_attempts, created_by
) VALUES (
    sqlc.arg(appointment_id), sqlc.arg(code_hash), sqlc.arg(expires_at),
    sqlc.arg(max_attempts), sqlc.arg(created_by)
)
RETURNING *;

-- name: GetActiveAppointmentOTP :one
SELECT * FROM appointment_otps
WHERE appointment_id = sqlc.arg(appointment_id)
  AND used_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now()
  AND failed_attempts < max_attempts
FOR UPDATE;

-- name: IncrementAppointmentOTPFailure :execrows
UPDATE appointment_otps
SET failed_attempts = failed_attempts + 1
WHERE id = sqlc.arg(id)
  AND used_at IS NULL
  AND revoked_at IS NULL
  AND failed_attempts < max_attempts;

-- name: ConsumeAppointmentOTP :execrows
UPDATE appointment_otps
SET used_at = now()
WHERE id = sqlc.arg(id)
  AND used_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now()
  AND failed_attempts < max_attempts;

-- name: RevokeAppointmentOTP :execrows
UPDATE appointment_otps
SET revoked_at = now()
WHERE appointment_id = sqlc.arg(appointment_id)
  AND used_at IS NULL
  AND revoked_at IS NULL;
