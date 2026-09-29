-- name: UpsertAnimalPreference :one
INSERT INTO animal_preferences (user_id, animal_id, preference)
VALUES (sqlc.arg(user_id), sqlc.arg(animal_id), sqlc.arg(preference))
ON CONFLICT (user_id, animal_id) DO UPDATE
SET preference = EXCLUDED.preference
RETURNING *;

-- name: GetAnimalPreference :one
SELECT * FROM animal_preferences
WHERE user_id = sqlc.arg(user_id) AND animal_id = sqlc.arg(animal_id);

-- name: ListPreferredAnimals :many
SELECT a.*, p.preference, p.updated_at AS preference_updated_at,
       o.display_name AS organization_name, o.slug AS organization_slug
FROM animal_preferences p
JOIN animals a ON a.id = p.animal_id
LEFT JOIN organizations o ON o.id = a.responsible_organization_id
WHERE p.user_id = sqlc.arg(user_id)
  AND p.preference = sqlc.arg(preference)
  AND a.deleted_at IS NULL
ORDER BY p.updated_at DESC, a.id
LIMIT sqlc.arg(page_size)::int OFFSET sqlc.arg(page_offset)::int;

-- name: DeleteAnimalPreference :execrows
DELETE FROM animal_preferences
WHERE user_id = sqlc.arg(user_id) AND animal_id = sqlc.arg(animal_id);
