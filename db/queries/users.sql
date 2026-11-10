-- name: CreateUser :one
INSERT INTO users (
    email, password_hash, display_name, phone, city, state_code, country_code,
    latitude, longitude, terms_accepted_at, privacy_accepted_at
) VALUES (
    sqlc.arg(email), sqlc.arg(password_hash), sqlc.arg(display_name), sqlc.narg(phone),
    sqlc.narg(city), sqlc.narg(state_code), sqlc.arg(country_code),
    sqlc.narg(latitude), sqlc.narg(longitude), sqlc.arg(terms_accepted_at),
    sqlc.arg(privacy_accepted_at)
)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetActiveUserByEmail :one
SELECT * FROM users
WHERE email = sqlc.arg(email)
  AND deleted_at IS NULL
  AND deactivated_at IS NULL;

-- name: UpdateUserProfile :one
UPDATE users
SET display_name = sqlc.arg(display_name),
    phone = sqlc.narg(phone),
    city = sqlc.narg(city),
    state_code = sqlc.narg(state_code),
    country_code = sqlc.arg(country_code),
    latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL AND deactivated_at IS NULL
RETURNING *;

-- name: MarkUserEmailVerified :one
UPDATE users
SET email_verified_at = COALESCE(email_verified_at, now())
WHERE id = sqlc.arg(id) AND deleted_at IS NULL
RETURNING *;

-- name: UpdateUserPassword :execrows
UPDATE users
SET password_hash = sqlc.arg(password_hash)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL AND deactivated_at IS NULL;

-- name: DeactivateUser :execrows
UPDATE users
SET deactivated_at = now()
WHERE id = sqlc.arg(id) AND deactivated_at IS NULL AND deleted_at IS NULL;

-- name: SoftDeleteUser :execrows
UPDATE users
SET deleted_at = now(), deactivated_at = COALESCE(deactivated_at, now())
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GrantUserRole :exec
INSERT INTO user_roles (user_id, role_id, granted_by)
SELECT sqlc.arg(user_id), id, sqlc.narg(granted_by)
FROM roles
WHERE key = sqlc.arg(role_key)
ON CONFLICT (user_id, role_id) DO NOTHING;

-- name: RevokeUserRole :execrows
DELETE FROM user_roles ur
USING roles r
WHERE ur.role_id = r.id
  AND ur.user_id = sqlc.arg(user_id)
  AND r.key = sqlc.arg(role_key);

-- name: ListUserRoles :many
SELECT r.*
FROM roles r
JOIN user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = sqlc.arg(user_id)
ORDER BY r.key;
