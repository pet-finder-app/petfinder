ALTER TABLE animals DROP CONSTRAINT animals_species_check;
ALTER TABLE animals ADD CONSTRAINT animals_species_check CHECK(species IN ('dog','cat','bird','rabbit','reptile','small_mammal','other'));
ALTER TABLE animals DROP CONSTRAINT animals_status_check;
ALTER TABLE animals ADD CONSTRAINT animals_status_check CHECK(status IN ('pending_organization','available','adoption_in_progress','adopted','unavailable','under_review','archived'));
ALTER TABLE animals ADD COLUMN attributes jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(attributes)='object');
CREATE TABLE animal_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, animal_id uuid NOT NULL REFERENCES animals(id),
 actor_id uuid NOT NULL REFERENCES users(id), action text NOT NULL,
 reason text NOT NULL, previous_organization_id uuid REFERENCES organizations(id), organization_id uuid REFERENCES organizations(id),
 from_status text NOT NULL, to_status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);

DO $$ DECLARE constraint_name text; BEGIN
 FOR constraint_name IN SELECT conname FROM pg_constraint WHERE conrelid='animals'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%pending_organization%' AND pg_get_constraintdef(oid) LIKE '%responsible_organization_id%'
 LOOP EXECUTE format('ALTER TABLE animals DROP CONSTRAINT %I',constraint_name); END LOOP;
END $$;
ALTER TABLE animals ADD CONSTRAINT animals_organization_status_check CHECK(status IN ('pending_organization','archived') OR responsible_organization_id IS NOT NULL);
