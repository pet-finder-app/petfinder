-- Totals for paginated collections. Each query mirrors the filters of the
-- list query named in its comment; keep them in sync.

-- name: CountOrganizationApplications :one
-- ListOrganizationApplications
SELECT count(*)::int FROM adoption_applications aa
WHERE aa.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR aa.status = sqlc.narg(status))
  AND (sqlc.narg(animal_id)::uuid IS NULL OR aa.animal_id = sqlc.narg(animal_id));

-- name: CountAdopterApplications :one
-- ListAdopterApplications
SELECT count(*)::int FROM adoption_applications
WHERE adopter_id = sqlc.arg(adopter_id)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: CountOrganizationConversations :one
-- ListOrganizationConversations
SELECT count(*)::int FROM conversations c
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status));

-- name: CountUserConversations :one
-- ListUserConversations
SELECT count(*)::int FROM conversations WHERE counterparty_user_id = sqlc.arg(user_id);

-- name: CountConversationMessages :one
-- ListMessagesBeforeCursor (all visible messages, independent of the cursor)
SELECT count(*)::int FROM messages m
WHERE m.conversation_id = sqlc.arg(conversation_id)
  AND m.deleted_at IS NULL
  AND (sqlc.arg(include_moderated)::boolean OR m.moderation_status IN ('visible', 'flagged'));

-- name: CountMemberOrganizations :one
-- ListMemberOrganizations
SELECT count(*)::int FROM organizations o
JOIN organization_members om ON om.organization_id = o.id
WHERE om.user_id = sqlc.arg(user_id)
  AND om.status IN ('invited', 'active')
  AND o.deactivated_at IS NULL;

-- name: CountAnimalsBySubmitter :one
-- ListAnimalsBySubmitter
SELECT count(*)::int FROM animals
WHERE submitted_by = sqlc.arg(submitted_by) AND deleted_at IS NULL;

-- name: CountAnimalsByOrganization :one
-- ListAnimalsByOrganization
SELECT count(*)::int FROM animals
WHERE responsible_organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: CountVerifiedOrganizations :one
-- ListVerifiedOrganizations
SELECT count(*)::int FROM organizations
WHERE verification_status = 'verified'
  AND deactivated_at IS NULL
  AND (sqlc.narg(state_code)::text IS NULL OR state_code = sqlc.narg(state_code))
  AND (sqlc.narg(city)::text IS NULL OR city = sqlc.narg(city));

-- name: CountOpenReports :one
-- ListOpenReports
SELECT count(*)::int FROM reports r
WHERE r.status NOT IN ('resolved', 'dismissed')
  AND (sqlc.narg(organization_id)::uuid IS NULL OR r.organization_id = sqlc.narg(organization_id))
  AND (sqlc.narg(assigned_to)::uuid IS NULL OR r.assigned_to = sqlc.narg(assigned_to));

-- name: CountAvailableAnimals :one
-- ListAvailableAnimals
SELECT count(*)::int FROM animals a
JOIN organizations o ON o.id = a.responsible_organization_id
WHERE a.status = 'available'
  AND a.deleted_at IS NULL
  AND o.verification_status = 'verified'
  AND o.deactivated_at IS NULL
  AND (sqlc.narg(species)::text IS NULL OR a.species = sqlc.narg(species))
  AND (sqlc.narg(size)::text IS NULL OR a.size = sqlc.narg(size))
  AND (sqlc.narg(state_code)::text IS NULL OR a.state_code = sqlc.narg(state_code))
  AND (sqlc.narg(city)::text IS NULL OR a.city = sqlc.narg(city))
  AND (sqlc.narg(organization_id)::uuid IS NULL OR a.responsible_organization_id = sqlc.narg(organization_id))
  AND NOT EXISTS (
      SELECT 1 FROM animal_preferences p
      WHERE p.user_id = sqlc.narg(viewer_id)
        AND p.animal_id = a.id
        AND p.preference = 'not_interested'
  );

-- name: CountPreferredAnimals :one
-- ListPreferredAnimals
SELECT count(*)::int FROM animal_preferences p
JOIN animals a ON a.id = p.animal_id
WHERE p.user_id = sqlc.arg(user_id)
  AND p.preference = sqlc.arg(preference)
  AND a.deleted_at IS NULL;
