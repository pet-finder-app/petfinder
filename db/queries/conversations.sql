-- name: CreateAdopterConversation :one
INSERT INTO conversations (
    organization_id, kind, animal_id, application_id, counterparty_user_id
)
SELECT aa.organization_id, 'adopter_organization', aa.animal_id, aa.id, aa.adopter_id
FROM adoption_applications aa
WHERE aa.id = sqlc.arg(application_id)
RETURNING *;

-- name: CreateAnimalOwnerConversation :one
INSERT INTO conversations (
    organization_id, kind, animal_id, counterparty_user_id
)
SELECT a.responsible_organization_id, 'owner_organization', a.id, a.submitted_by
FROM animals a
WHERE a.id = sqlc.arg(animal_id)
  AND a.responsible_organization_id IS NOT NULL
  AND a.deleted_at IS NULL
RETURNING *;

-- name: GetConversationByID :one
SELECT * FROM conversations WHERE id = sqlc.arg(id);

-- name: ListUserConversations :many
SELECT c.*, a.name AS animal_name, o.display_name AS organization_name,
       latest.body AS last_message_body, latest.created_at AS last_message_at
FROM conversations c
JOIN animals a ON a.id = c.animal_id
JOIN organizations o ON o.id = c.organization_id
LEFT JOIN LATERAL (
    SELECT m.body, m.created_at
    FROM messages m
    WHERE m.conversation_id = c.id AND m.deleted_at IS NULL
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
) latest ON true
WHERE c.counterparty_user_id = sqlc.arg(user_id)
ORDER BY COALESCE(latest.created_at, c.created_at) DESC, c.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ListOrganizationConversations :many
SELECT c.*, a.name AS animal_name, u.display_name AS counterparty_name,
       latest.body AS last_message_body, latest.created_at AS last_message_at
FROM conversations c
JOIN animals a ON a.id = c.animal_id
JOIN users u ON u.id = c.counterparty_user_id
LEFT JOIN LATERAL (
    SELECT m.body, m.created_at
    FROM messages m
    WHERE m.conversation_id = c.id AND m.deleted_at IS NULL
    ORDER BY m.created_at DESC, m.id DESC
    LIMIT 1
) latest ON true
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status))
ORDER BY COALESCE(latest.created_at, c.created_at) DESC, c.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: CloseConversation :one
UPDATE conversations
SET status = sqlc.arg(status), closed_by = sqlc.arg(closed_by), closed_at = now()
WHERE id = sqlc.arg(id) AND status = 'open'
RETURNING *;

-- name: CreateMessage :one
INSERT INTO messages (conversation_id, sender_id, body)
VALUES (sqlc.arg(conversation_id), sqlc.arg(sender_id), sqlc.arg(body))
RETURNING *;

-- name: ListConversationMessages :many
SELECT * FROM messages
WHERE conversation_id = sqlc.arg(conversation_id)
  AND (sqlc.arg(include_moderated)::boolean OR moderation_status IN ('visible', 'flagged'))
  AND (sqlc.narg(before_created_at)::timestamptz IS NULL OR created_at < sqlc.narg(before_created_at))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size)::int;

-- name: EditMessage :one
UPDATE messages
SET body = sqlc.arg(body), edited_at = now()
WHERE id = sqlc.arg(id)
  AND sender_id = sqlc.arg(sender_id)
  AND deleted_at IS NULL
  AND moderation_status = 'visible'
RETURNING *;

-- name: ModerateMessage :one
UPDATE messages
SET moderation_status = sqlc.arg(moderation_status),
    moderation_reason = sqlc.arg(moderation_reason),
    moderated_by = sqlc.arg(moderated_by),
    moderated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: SoftDeleteMessage :execrows
UPDATE messages
SET deleted_at = now()
WHERE id = sqlc.arg(id) AND sender_id = sqlc.arg(sender_id) AND deleted_at IS NULL;
