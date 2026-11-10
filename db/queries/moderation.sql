-- name: CreateReport :one
INSERT INTO reports (
    reporter_id, organization_id, animal_id, application_id,
    conversation_id, message_id, category, description, priority
) VALUES (
    sqlc.arg(reporter_id), sqlc.narg(organization_id), sqlc.narg(animal_id),
    sqlc.narg(application_id), sqlc.narg(conversation_id), sqlc.narg(message_id),
    sqlc.arg(category), sqlc.arg(description), sqlc.arg(priority)
)
RETURNING *;

-- name: GetReportByID :one
SELECT * FROM reports WHERE id = sqlc.arg(id);

-- name: ListOpenReports :many
SELECT r.*, u.display_name AS reporter_name
FROM reports r
JOIN users u ON u.id = r.reporter_id
WHERE r.status NOT IN ('resolved', 'dismissed')
  AND (sqlc.narg(organization_id)::uuid IS NULL OR r.organization_id = sqlc.narg(organization_id))
  AND (sqlc.narg(assigned_to)::uuid IS NULL OR r.assigned_to = sqlc.narg(assigned_to))
ORDER BY
    CASE r.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
    r.created_at,
    r.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: AssignReport :one
UPDATE reports
SET assigned_to = sqlc.arg(assigned_to),
    status = CASE WHEN status = 'open' THEN 'triaged' ELSE status END
WHERE id = sqlc.arg(id) AND status NOT IN ('resolved', 'dismissed')
RETURNING *;

-- name: SetReportStatus :one
UPDATE reports
SET status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND status NOT IN ('resolved', 'dismissed')
RETURNING *;

-- name: ResolveReport :one
UPDATE reports
SET status = sqlc.arg(status), resolution = sqlc.arg(resolution),
    resolved_by = sqlc.arg(resolved_by), resolved_at = now()
WHERE id = sqlc.arg(id)
  AND status NOT IN ('resolved', 'dismissed')
  AND sqlc.arg(status)::text IN ('resolved', 'dismissed')
RETURNING *;
