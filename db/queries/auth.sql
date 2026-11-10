-- name: CreateRefreshSession :one
INSERT INTO refresh_sessions (
    user_id, token_hash, user_agent, ip_address, expires_at
) VALUES (
    sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.narg(user_agent),
    sqlc.narg(ip_address), sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetActiveRefreshSessionByTokenHash :one
SELECT * FROM refresh_sessions
WHERE token_hash = sqlc.arg(token_hash)
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: TouchRefreshSession :execrows
UPDATE refresh_sessions
SET last_used_at = now()
WHERE id = sqlc.arg(id) AND revoked_at IS NULL AND expires_at > now();

-- name: RotateRefreshSession :execrows
UPDATE refresh_sessions
SET revoked_at = now(),
    revoke_reason = 'rotated',
    replaced_by_id = sqlc.arg(replaced_by_id),
    last_used_at = now()
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL
  AND replaced_by_id IS NULL
  AND expires_at > now();

-- name: RevokeRefreshSession :execrows
UPDATE refresh_sessions
SET revoked_at = now(), revoke_reason = sqlc.arg(revoke_reason)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshSessions :execrows
UPDATE refresh_sessions
SET revoked_at = now(), revoke_reason = sqlc.arg(revoke_reason)
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshSessions :execrows
DELETE FROM refresh_sessions
WHERE expires_at < sqlc.arg(expired_before);

-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at))
RETURNING *;

-- name: GetActivePasswordResetTokenByHash :one
SELECT * FROM password_reset_tokens
WHERE token_hash = sqlc.arg(token_hash)
  AND used_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: ConsumePasswordResetToken :execrows
UPDATE password_reset_tokens
SET used_at = now()
WHERE id = sqlc.arg(id)
  AND used_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: RevokeUserPasswordResetTokens :execrows
UPDATE password_reset_tokens
SET revoked_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND used_at IS NULL
  AND revoked_at IS NULL;

-- name: DeleteExpiredPasswordResetTokens :execrows
DELETE FROM password_reset_tokens
WHERE expires_at < sqlc.arg(expired_before);
