-- name: CreateOrganization :one
INSERT INTO organizations (
    legal_name, display_name, slug, registration_number, description,
    official_email, official_phone, website_url, city, state_code,
    country_code, latitude, longitude, created_by
) VALUES (
    sqlc.arg(legal_name), sqlc.arg(display_name), sqlc.arg(slug),
    sqlc.narg(registration_number), sqlc.arg(description), sqlc.arg(official_email),
    sqlc.narg(official_phone), sqlc.narg(website_url), sqlc.arg(city),
    sqlc.arg(state_code), sqlc.arg(country_code), sqlc.narg(latitude),
    sqlc.narg(longitude), sqlc.arg(created_by)
)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations WHERE id = sqlc.arg(id);

-- name: GetPublicOrganizationBySlug :one
SELECT o.*,
       count(a.id) FILTER (WHERE a.status = 'available' AND a.deleted_at IS NULL) AS available_animals
FROM organizations o
LEFT JOIN animals a ON a.responsible_organization_id = o.id
WHERE o.slug = sqlc.arg(slug)
  AND o.verification_status = 'verified'
  AND o.deactivated_at IS NULL
GROUP BY o.id;

-- name: ListVerifiedOrganizations :many
SELECT * FROM organizations
WHERE verification_status = 'verified'
  AND deactivated_at IS NULL
  AND (sqlc.narg(state_code)::text IS NULL OR state_code = sqlc.narg(state_code))
  AND (sqlc.narg(city)::text IS NULL OR city = sqlc.narg(city))
ORDER BY display_name, id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: UpdateOrganizationProfile :one
UPDATE organizations
SET display_name = sqlc.arg(display_name),
    description = sqlc.arg(description),
    official_email = sqlc.arg(official_email),
    official_phone = sqlc.narg(official_phone),
    website_url = sqlc.narg(website_url),
    city = sqlc.arg(city),
    state_code = sqlc.arg(state_code),
    country_code = sqlc.arg(country_code),
    latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude)
WHERE id = sqlc.arg(id) AND deactivated_at IS NULL
RETURNING *;

-- name: SetOrganizationVerification :one
UPDATE organizations
SET verification_status = sqlc.arg(verification_status),
    verification_notes = sqlc.narg(verification_notes),
    verified_by = CASE WHEN sqlc.arg(verification_status)::text = 'verified' THEN sqlc.arg(actor_id) ELSE verified_by END,
    verified_at = CASE WHEN sqlc.arg(verification_status)::text = 'verified' THEN now() ELSE verified_at END
WHERE id = sqlc.arg(id) AND deactivated_at IS NULL
RETURNING *;

-- name: DeactivateOrganization :execrows
UPDATE organizations
SET deactivated_at = now(),
    verification_status = CASE WHEN verification_status = 'verified' THEN 'suspended' ELSE verification_status END
WHERE id = sqlc.arg(id) AND deactivated_at IS NULL;

-- name: InviteOrganizationMember :one
INSERT INTO organization_members (
    organization_id, user_id, member_role, can_manage_members,
    can_publish_animals, can_manage_applications, invited_by
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(member_role),
    sqlc.arg(can_manage_members), sqlc.arg(can_publish_animals),
    sqlc.arg(can_manage_applications), sqlc.arg(invited_by)
)
RETURNING *;

-- name: AcceptOrganizationMembership :one
UPDATE organization_members
SET status = 'active', joined_at = now(), deactivated_at = NULL
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND status = 'invited'
RETURNING *;

-- name: UpdateOrganizationMemberPermissions :one
UPDATE organization_members
SET member_role = sqlc.arg(member_role),
    can_manage_members = sqlc.arg(can_manage_members),
    can_publish_animals = sqlc.arg(can_publish_animals),
    can_manage_applications = sqlc.arg(can_manage_applications)
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status = 'active'
RETURNING *;

-- name: RemoveOrganizationMember :execrows
UPDATE organization_members
SET status = 'removed', deactivated_at = now()
WHERE id = sqlc.arg(id) AND organization_id = sqlc.arg(organization_id) AND status IN ('invited', 'active');

-- name: GetActiveOrganizationMember :one
SELECT * FROM organization_members
WHERE organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'active';

-- name: ListOrganizationMembers :many
SELECT om.*, u.display_name, u.email
FROM organization_members om
JOIN users u ON u.id = om.user_id
WHERE om.organization_id = sqlc.arg(organization_id)
  AND (sqlc.narg(status)::text IS NULL OR om.status = sqlc.narg(status))
ORDER BY om.created_at, om.id;
