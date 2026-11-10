-- name: CreateAuditLog :one
INSERT INTO audit_log (
    actor_user_id, organization_id, action, entity_type, entity_id,
    old_values, new_values, metadata, request_id, ip_address
) VALUES (
    sqlc.narg(actor_user_id), sqlc.narg(organization_id), sqlc.arg(action),
    sqlc.arg(entity_type), sqlc.narg(entity_id), sqlc.narg(old_values),
    sqlc.narg(new_values), sqlc.arg(metadata), sqlc.narg(request_id),
    sqlc.narg(ip_address)
)
RETURNING *;

-- name: ListEntityAuditLog :many
SELECT * FROM audit_log
WHERE entity_type = sqlc.arg(entity_type)
  AND entity_id = sqlc.arg(entity_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ListActorAuditLog :many
SELECT * FROM audit_log
WHERE actor_user_id = sqlc.arg(actor_user_id)
  AND created_at >= sqlc.arg(created_after)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;
