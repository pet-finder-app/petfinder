-- name: CreatePrivateAnimal :one
INSERT INTO animals (
    submitted_by, responsible_organization_id, source, name, species, breed,
    sex, size, birth_date, approximate_age_months, description, health_notes,
    behavior_notes, vaccinated, neutered, special_needs, city, state_code,
    country_code, latitude, longitude
) VALUES (
    sqlc.arg(submitted_by), sqlc.narg(responsible_organization_id), 'private_submission',
    sqlc.arg(name), sqlc.arg(species), sqlc.narg(breed), sqlc.arg(sex), sqlc.arg(size),
    sqlc.narg(birth_date), sqlc.narg(approximate_age_months), sqlc.arg(description),
    sqlc.arg(health_notes), sqlc.arg(behavior_notes), sqlc.narg(vaccinated),
    sqlc.narg(neutered), sqlc.arg(special_needs), sqlc.arg(city), sqlc.arg(state_code),
    sqlc.arg(country_code), sqlc.narg(latitude), sqlc.narg(longitude)
)
RETURNING *;

-- name: CreateOrganizationAnimal :one
INSERT INTO animals (
    submitted_by, responsible_organization_id, source, status,
    organization_review_status, reviewed_by, reviewed_at, name, species,
    breed, sex, size, birth_date, approximate_age_months, description,
    health_notes, behavior_notes, vaccinated, neutered, special_needs,
    city, state_code, country_code, latitude, longitude, published_at
) VALUES (
    sqlc.arg(submitted_by), sqlc.arg(responsible_organization_id), 'organization',
    'available', 'accepted', sqlc.arg(submitted_by), now(), sqlc.arg(name),
    sqlc.arg(species), sqlc.narg(breed), sqlc.arg(sex), sqlc.arg(size),
    sqlc.narg(birth_date), sqlc.narg(approximate_age_months), sqlc.arg(description),
    sqlc.arg(health_notes), sqlc.arg(behavior_notes), sqlc.narg(vaccinated),
    sqlc.narg(neutered), sqlc.arg(special_needs), sqlc.arg(city), sqlc.arg(state_code),
    sqlc.arg(country_code), sqlc.narg(latitude), sqlc.narg(longitude), now()
)
RETURNING *;

-- name: GetAnimalByID :one
SELECT * FROM animals WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

-- name: GetAnimalWithOrganization :one
SELECT a.*, o.display_name AS organization_name, o.slug AS organization_slug,
       o.verification_status AS organization_verification_status
FROM animals a
LEFT JOIN organizations o ON o.id = a.responsible_organization_id
WHERE a.id = sqlc.arg(id) AND a.deleted_at IS NULL;

-- name: ListAvailableAnimals :many
SELECT a.*, o.display_name AS organization_name, o.slug AS organization_slug,
       CASE
           WHEN sqlc.narg(viewer_latitude)::numeric IS NULL
             OR sqlc.narg(viewer_longitude)::numeric IS NULL
             OR a.latitude IS NULL OR a.longitude IS NULL THEN NULL
           ELSE power(a.latitude - sqlc.narg(viewer_latitude), 2)
              + power(a.longitude - sqlc.narg(viewer_longitude), 2)
       END AS distance_score
FROM animals a
JOIN organizations o ON o.id = a.responsible_organization_id
WHERE a.status = 'available'
  AND a.deleted_at IS NULL
  AND o.verification_status = 'verified'
  AND o.deactivated_at IS NULL
  AND (sqlc.narg(species)::text IS NULL OR a.species = sqlc.narg(species))
  AND (sqlc.narg(size)::text IS NULL OR a.size = sqlc.narg(size))
  AND (sqlc.narg(state_code)::text IS NULL OR a.state_code = sqlc.narg(state_code))
  AND (sqlc.narg(city)::text IS NULL OR a.city = sqlc.narg(city))
  AND NOT EXISTS (
      SELECT 1 FROM animal_preferences p
      WHERE p.user_id = sqlc.narg(viewer_id)
        AND p.animal_id = a.id
        AND p.preference = 'not_interested'
  )
ORDER BY distance_score ASC NULLS LAST, a.published_at DESC, a.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ListAnimalsByOrganization :many
SELECT * FROM animals
WHERE responsible_organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC, id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: ListAnimalsBySubmitter :many
SELECT * FROM animals
WHERE submitted_by = sqlc.arg(submitted_by)
  AND deleted_at IS NULL
ORDER BY created_at DESC, id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: AssignAnimalOrganization :one
UPDATE animals
SET responsible_organization_id = sqlc.arg(organization_id)
WHERE id = sqlc.arg(id)
  AND source = 'private_submission'
  AND status = 'pending_organization'
  AND organization_review_status IN ('pending', 'changes_requested')
  AND deleted_at IS NULL
RETURNING *;

-- name: ReviewAnimalSubmission :one
UPDATE animals
SET organization_review_status = sqlc.arg(review_status),
    organization_review_notes = sqlc.narg(review_notes),
    reviewed_by = sqlc.arg(reviewed_by),
    reviewed_at = now(),
    status = CASE
        WHEN sqlc.arg(review_status)::text = 'accepted' THEN 'available'
        ELSE 'pending_organization'
    END,
    published_at = CASE WHEN sqlc.arg(review_status)::text = 'accepted' THEN now() ELSE NULL END
WHERE id = sqlc.arg(id)
  AND responsible_organization_id = sqlc.arg(organization_id)
  AND source = 'private_submission'
  AND status = 'pending_organization'
  AND deleted_at IS NULL
RETURNING *;

-- name: UpdateAnimalDetails :one
UPDATE animals
SET name = sqlc.arg(name), species = sqlc.arg(species), breed = sqlc.narg(breed),
    sex = sqlc.arg(sex), size = sqlc.arg(size), birth_date = sqlc.narg(birth_date),
    approximate_age_months = sqlc.narg(approximate_age_months),
    description = sqlc.arg(description), health_notes = sqlc.arg(health_notes),
    behavior_notes = sqlc.arg(behavior_notes), vaccinated = sqlc.narg(vaccinated),
    neutered = sqlc.narg(neutered), special_needs = sqlc.arg(special_needs),
    city = sqlc.arg(city), state_code = sqlc.arg(state_code),
    country_code = sqlc.arg(country_code), latitude = sqlc.narg(latitude),
    longitude = sqlc.narg(longitude)
WHERE id = sqlc.arg(id) AND deleted_at IS NULL AND status <> 'adopted'
RETURNING *;

-- name: SetAnimalStatus :one
UPDATE animals
SET status = sqlc.arg(status),
    unavailable_reason = CASE
        WHEN sqlc.arg(status)::text IN ('unavailable', 'under_review') THEN sqlc.arg(reason)
        ELSE unavailable_reason
    END,
    adopted_at = CASE WHEN sqlc.arg(status)::text = 'adopted' THEN now() ELSE adopted_at END
WHERE id = sqlc.arg(id)
  AND responsible_organization_id = sqlc.arg(organization_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteAnimal :execrows
UPDATE animals
SET deleted_at = now()
WHERE id = sqlc.arg(id) AND submitted_by = sqlc.arg(submitted_by) AND deleted_at IS NULL;

-- name: AddAnimalImage :one
INSERT INTO animal_images (animal_id, storage_key, alt_text, position)
VALUES (sqlc.arg(animal_id), sqlc.arg(storage_key), sqlc.arg(alt_text), sqlc.arg(position))
RETURNING *;

-- name: ListAnimalImages :many
SELECT * FROM animal_images
WHERE animal_id = sqlc.arg(animal_id)
  AND removed_at IS NULL
  AND (sqlc.arg(include_pending)::boolean OR moderation_status = 'approved')
ORDER BY position, created_at, id;

-- name: ModerateAnimalImage :one
UPDATE animal_images
SET moderation_status = sqlc.arg(moderation_status),
    moderated_by = sqlc.arg(moderated_by),
    moderated_at = now()
WHERE id = sqlc.arg(id) AND removed_at IS NULL
RETURNING *;

-- name: RemoveAnimalImage :execrows
UPDATE animal_images
SET removed_at = now()
WHERE id = sqlc.arg(id) AND animal_id = sqlc.arg(animal_id) AND removed_at IS NULL;
