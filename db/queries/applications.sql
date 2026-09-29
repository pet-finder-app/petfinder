-- name: CreateAdoptionApplication :one
INSERT INTO adoption_applications (
    animal_id, adopter_id, organization_id, applicant_message
)
SELECT a.id, sqlc.arg(adopter_id), a.responsible_organization_id, sqlc.arg(applicant_message)
FROM animals a
WHERE a.id = sqlc.arg(animal_id)
  AND a.status = 'available'
  AND a.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM animal_preferences p
      WHERE p.user_id = sqlc.arg(adopter_id)
        AND p.animal_id = a.id
        AND p.preference = 'liked'
  )
RETURNING *;

-- name: GetAdoptionApplicationByID :one
SELECT aa.*, a.name AS animal_name, a.status AS animal_status,
       u.display_name AS adopter_name, u.email AS adopter_email,
       o.display_name AS organization_name
FROM adoption_applications aa
JOIN animals a ON a.id = aa.animal_id
JOIN users u ON u.id = aa.adopter_id
JOIN organizations o ON o.id = aa.organization_id
WHERE aa.id = sqlc.arg(id);

-- name: ListOrganizationApplications :many
SELECT aa.*, a.name AS animal_name, u.display_name AS adopter_name
FROM adoption_applications aa
JOIN animals a ON a.id = aa.animal_id
JOIN users u ON u.id = aa.adopter_id
WHERE aa.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR aa.status = sqlc.narg(status))
  AND (sqlc.narg(animal_id)::uuid IS NULL OR aa.animal_id = sqlc.narg(animal_id))
ORDER BY aa.created_at DESC, aa.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ListAdopterApplications :many
SELECT aa.*, a.name AS animal_name, o.display_name AS organization_name
FROM adoption_applications aa
JOIN animals a ON a.id = aa.animal_id
JOIN organizations o ON o.id = aa.organization_id
WHERE aa.adopter_id = sqlc.arg(adopter_id)
ORDER BY aa.created_at DESC, aa.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: SetApplicationStatus :one
UPDATE adoption_applications
SET status = sqlc.arg(status),
    decision_reason = CASE
        WHEN sqlc.arg(status)::text IN ('approved', 'rejected') THEN sqlc.narg(reason)
        ELSE decision_reason
    END,
    decided_by = CASE
        WHEN sqlc.arg(status)::text IN ('approved', 'rejected') THEN sqlc.arg(actor_id)
        ELSE decided_by
    END,
    decided_at = CASE
        WHEN sqlc.arg(status)::text IN ('approved', 'rejected') THEN now()
        ELSE decided_at
    END,
    withdrawn_at = CASE WHEN sqlc.arg(status)::text = 'withdrawn' THEN now() ELSE withdrawn_at END,
    completed_at = CASE WHEN sqlc.arg(status)::text = 'completed' THEN now() ELSE completed_at END
WHERE id = sqlc.arg(id)
  AND organization_id = sqlc.arg(organization_id)
RETURNING *;

-- name: WithdrawApplication :one
UPDATE adoption_applications
SET status = 'withdrawn', withdrawn_at = now()
WHERE id = sqlc.arg(id)
  AND adopter_id = sqlc.arg(adopter_id)
  AND status IN ('submitted', 'screening', 'more_information_requested')
RETURNING *;

-- name: AddApplicationStatusHistory :one
INSERT INTO application_status_history (
    application_id, from_status, to_status, reason, changed_by
) VALUES (
    sqlc.arg(application_id), sqlc.narg(from_status), sqlc.arg(to_status),
    sqlc.narg(reason), sqlc.narg(changed_by)
)
RETURNING *;

-- name: ListApplicationStatusHistory :many
SELECT * FROM application_status_history
WHERE application_id = sqlc.arg(application_id)
ORDER BY created_at, id;

-- name: CreateApplicationScreening :one
INSERT INTO application_screenings (
    application_id, version, questions, requested_by
) VALUES (
    sqlc.arg(application_id), sqlc.arg(version), sqlc.arg(questions), sqlc.arg(requested_by)
)
RETURNING *;

-- name: GetLatestApplicationScreening :one
SELECT * FROM application_screenings
WHERE application_id = sqlc.arg(application_id)
ORDER BY version DESC
LIMIT 1;

-- name: SubmitApplicationScreening :one
UPDATE application_screenings
SET answers = sqlc.arg(answers), status = 'submitted', submitted_at = now()
WHERE id = sqlc.arg(id) AND status = 'requested'
RETURNING *;

-- name: ReviewApplicationScreening :one
UPDATE application_screenings
SET status = 'reviewed', reviewed_by = sqlc.arg(reviewed_by), reviewed_at = now(),
    outcome = sqlc.arg(outcome), private_notes = sqlc.narg(private_notes)
WHERE id = sqlc.arg(id) AND status = 'submitted'
RETURNING *;

-- name: CreatePostAdoptionFollowup :one
INSERT INTO post_adoption_followups (
    application_id, organization_id, occurred_at, contact_method,
    notes, welfare_status, recorded_by
) VALUES (
    sqlc.arg(application_id), sqlc.arg(organization_id), sqlc.arg(occurred_at),
    sqlc.arg(contact_method), sqlc.arg(notes), sqlc.arg(welfare_status),
    sqlc.arg(recorded_by)
)
RETURNING *;

-- name: ListPostAdoptionFollowups :many
SELECT * FROM post_adoption_followups
WHERE application_id = sqlc.arg(application_id)
ORDER BY occurred_at DESC, id;
