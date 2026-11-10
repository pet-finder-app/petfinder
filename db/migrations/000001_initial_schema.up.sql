CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email citext NOT NULL UNIQUE,
    password_hash text NOT NULL,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 2 AND 120),
    phone text,
    city text,
    state_code varchar(8),
    country_code char(2) NOT NULL DEFAULT 'BR',
    latitude numeric(9, 6) CHECK (latitude BETWEEN -90 AND 90),
    longitude numeric(9, 6) CHECK (longitude BETWEEN -180 AND 180),
    terms_accepted_at timestamptz NOT NULL,
    privacy_accepted_at timestamptz NOT NULL,
    email_verified_at timestamptz,
    deactivated_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((latitude IS NULL) = (longitude IS NULL))
);

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key text NOT NULL UNIQUE CHECK (key ~ '^[a-z][a-z_]*$'),
    description text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO roles (id, key, description) VALUES
    ('00000000-0000-0000-0000-000000000001', 'adopter', 'Can discover animals and submit adoption applications'),
    ('00000000-0000-0000-0000-000000000002', 'advertiser', 'Can submit animals to an organization'),
    ('00000000-0000-0000-0000-000000000003', 'organization_member', 'Can represent an organization when membership is active'),
    ('00000000-0000-0000-0000-000000000004', 'platform_admin', 'Can verify organizations and moderate the platform');

CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE refresh_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) >= 32),
    user_agent text,
    ip_address inet,
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at timestamptz,
    revoke_reason text,
    replaced_by_id uuid REFERENCES refresh_sessions(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    CHECK (replaced_by_id IS NULL OR revoked_at IS NOT NULL)
);

CREATE TABLE password_reset_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) >= 32),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    CHECK (used_at IS NULL OR used_at >= created_at),
    CHECK (used_at IS NULL OR revoked_at IS NULL)
);

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    legal_name text NOT NULL CHECK (length(btrim(legal_name)) BETWEEN 2 AND 180),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 2 AND 120),
    slug citext NOT NULL UNIQUE CHECK (slug::text ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
    registration_number text,
    description text NOT NULL DEFAULT '',
    official_email citext NOT NULL,
    official_phone text,
    website_url text,
    city text NOT NULL,
    state_code varchar(8) NOT NULL,
    country_code char(2) NOT NULL DEFAULT 'BR',
    latitude numeric(9, 6) CHECK (latitude BETWEEN -90 AND 90),
    longitude numeric(9, 6) CHECK (longitude BETWEEN -180 AND 180),
    verification_status text NOT NULL DEFAULT 'pending'
        CHECK (verification_status IN ('pending', 'verified', 'rejected', 'suspended')),
    verification_notes text,
    verified_by uuid REFERENCES users(id) ON DELETE SET NULL,
    verified_at timestamptz,
    deactivated_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK (verification_status <> 'verified' OR (verified_at IS NOT NULL AND verified_by IS NOT NULL))
);

CREATE TABLE organization_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    member_role text NOT NULL CHECK (member_role IN ('owner', 'coordinator', 'representative', 'moderator')),
    status text NOT NULL DEFAULT 'invited' CHECK (status IN ('invited', 'active', 'declined', 'removed')),
    can_manage_members boolean NOT NULL DEFAULT false,
    can_publish_animals boolean NOT NULL DEFAULT false,
    can_manage_applications boolean NOT NULL DEFAULT false,
    invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
    joined_at timestamptz,
    deactivated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, user_id),
    CHECK (status <> 'active' OR joined_at IS NOT NULL),
    CHECK ((status IN ('declined', 'removed')) = (deactivated_at IS NOT NULL))
);

CREATE TABLE animals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    submitted_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    responsible_organization_id uuid REFERENCES organizations(id) ON DELETE RESTRICT,
    source text NOT NULL CHECK (source IN ('private_submission', 'organization')),
    status text NOT NULL DEFAULT 'pending_organization'
        CHECK (status IN ('pending_organization', 'available', 'adoption_in_progress', 'adopted', 'unavailable', 'under_review')),
    organization_review_status text NOT NULL DEFAULT 'pending'
        CHECK (organization_review_status IN ('pending', 'changes_requested', 'accepted', 'rejected')),
    organization_review_notes text,
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    species text NOT NULL CHECK (species IN ('dog', 'cat', 'other')),
    breed text,
    sex text NOT NULL DEFAULT 'unknown' CHECK (sex IN ('female', 'male', 'unknown')),
    size text NOT NULL CHECK (size IN ('small', 'medium', 'large', 'extra_large')),
    birth_date date,
    approximate_age_months integer CHECK (approximate_age_months >= 0),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 20 AND 5000),
    health_notes text NOT NULL DEFAULT '',
    behavior_notes text NOT NULL DEFAULT '',
    vaccinated boolean,
    neutered boolean,
    special_needs boolean NOT NULL DEFAULT false,
    city text NOT NULL,
    state_code varchar(8) NOT NULL,
    country_code char(2) NOT NULL DEFAULT 'BR',
    latitude numeric(9, 6) CHECK (latitude BETWEEN -90 AND 90),
    longitude numeric(9, 6) CHECK (longitude BETWEEN -180 AND 180),
    published_at timestamptz,
    unavailable_reason text,
    adopted_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK (birth_date IS NULL OR birth_date <= CURRENT_DATE),
    CHECK (birth_date IS NULL OR approximate_age_months IS NULL),
    CHECK (source <> 'organization' OR responsible_organization_id IS NOT NULL),
    CHECK (status = 'pending_organization' OR responsible_organization_id IS NOT NULL),
    CHECK (status <> 'available' OR (organization_review_status = 'accepted' AND published_at IS NOT NULL)),
    CHECK (status <> 'adopted' OR adopted_at IS NOT NULL),
    CHECK (status NOT IN ('unavailable', 'under_review') OR unavailable_reason IS NOT NULL),
    UNIQUE (id, responsible_organization_id)
);

CREATE TABLE animal_images (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    animal_id uuid NOT NULL REFERENCES animals(id) ON DELETE CASCADE,
    storage_key text NOT NULL UNIQUE,
    alt_text text NOT NULL DEFAULT '',
    position smallint NOT NULL DEFAULT 0 CHECK (position >= 0),
    moderation_status text NOT NULL DEFAULT 'pending'
        CHECK (moderation_status IN ('pending', 'approved', 'rejected')),
    moderated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    moderated_at timestamptz,
    removed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((moderation_status = 'pending') = (moderated_at IS NULL))
);

CREATE TABLE animal_preferences (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    animal_id uuid NOT NULL REFERENCES animals(id) ON DELETE CASCADE,
    preference text NOT NULL CHECK (preference IN ('liked', 'not_interested')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, animal_id)
);

CREATE TABLE adoption_applications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    animal_id uuid NOT NULL REFERENCES animals(id) ON DELETE RESTRICT,
    adopter_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'submitted'
        CHECK (status IN ('submitted', 'screening', 'more_information_requested', 'approved', 'rejected', 'withdrawn', 'completed', 'cancelled')),
    applicant_message text NOT NULL DEFAULT '',
    decision_reason text,
    decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
    decided_at timestamptz,
    withdrawn_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (animal_id, adopter_id),
    UNIQUE (id, organization_id),
    UNIQUE (id, animal_id, organization_id),
    FOREIGN KEY (animal_id, organization_id)
        REFERENCES animals (id, responsible_organization_id) ON DELETE RESTRICT,
    CHECK (status NOT IN ('approved', 'rejected', 'completed') OR decided_at IS NOT NULL),
    CHECK (status NOT IN ('approved', 'rejected', 'completed') OR decided_by IS NOT NULL),
    CHECK ((status = 'withdrawn') = (withdrawn_at IS NOT NULL)),
    CHECK ((status = 'completed') = (completed_at IS NOT NULL))
);

CREATE TABLE application_screenings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES adoption_applications(id) ON DELETE CASCADE,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    questions jsonb NOT NULL CHECK (jsonb_typeof(questions) = 'array'),
    answers jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(answers) = 'object'),
    status text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'submitted', 'reviewed')),
    requested_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    submitted_at timestamptz,
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    outcome text CHECK (outcome IN ('suitable', 'needs_review', 'not_suitable')),
    private_notes text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, version),
    CHECK (status = 'requested' OR submitted_at IS NOT NULL),
    CHECK ((status = 'reviewed') = (reviewed_at IS NOT NULL)),
    CHECK (status <> 'reviewed' OR (reviewed_by IS NOT NULL AND outcome IS NOT NULL))
);

CREATE TABLE application_status_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES adoption_applications(id) ON DELETE CASCADE,
    from_status text,
    to_status text NOT NULL,
    reason text,
    changed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE conversations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('adopter_organization', 'owner_organization')),
    animal_id uuid NOT NULL REFERENCES animals(id) ON DELETE RESTRICT,
    application_id uuid REFERENCES adoption_applications(id) ON DELETE RESTRICT,
    counterparty_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed', 'blocked')),
    closed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    closed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (animal_id, organization_id)
        REFERENCES animals (id, responsible_organization_id) ON DELETE RESTRICT,
    FOREIGN KEY (application_id, animal_id, organization_id)
        REFERENCES adoption_applications (id, animal_id, organization_id) ON DELETE RESTRICT,
    CHECK ((kind = 'adopter_organization') = (application_id IS NOT NULL)),
    CHECK ((status = 'open') = (closed_at IS NULL))
);

CREATE UNIQUE INDEX conversations_adopter_application_uidx
    ON conversations (application_id) WHERE kind = 'adopter_organization';
CREATE UNIQUE INDEX conversations_owner_animal_uidx
    ON conversations (animal_id, counterparty_user_id) WHERE kind = 'owner_organization';

CREATE TABLE messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    body text NOT NULL CHECK (length(btrim(body)) BETWEEN 1 AND 10000),
    moderation_status text NOT NULL DEFAULT 'visible'
        CHECK (moderation_status IN ('visible', 'flagged', 'hidden', 'removed')),
    moderation_reason text,
    moderated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    moderated_at timestamptz,
    edited_at timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (moderation_status IN ('visible', 'flagged') OR moderated_at IS NOT NULL)
);

CREATE TABLE appointments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES adoption_applications(id) ON DELETE RESTRICT,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('interview', 'home_visit', 'meet_and_greet', 'delivery', 'follow_up')),
    status text NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed', 'scheduled', 'completed', 'cancelled', 'no_show')),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    timezone text NOT NULL,
    location_name text NOT NULL,
    address text NOT NULL,
    latitude numeric(9, 6) CHECK (latitude BETWEEN -90 AND 90),
    longitude numeric(9, 6) CHECK (longitude BETWEEN -180 AND 180),
    instructions text NOT NULL DEFAULT '',
    otp_required boolean NOT NULL DEFAULT true,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    cancelled_by uuid REFERENCES users(id) ON DELETE SET NULL,
    cancelled_at timestamptz,
    cancellation_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (application_id, organization_id)
        REFERENCES adoption_applications (id, organization_id) ON DELETE RESTRICT,
    CHECK (ends_at > starts_at),
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

CREATE TABLE appointment_confirmations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_id uuid NOT NULL REFERENCES appointments(id) ON DELETE CASCADE,
    party text NOT NULL CHECK (party IN ('organization', 'adopter', 'advertiser')),
    confirmed_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmation text NOT NULL CHECK (confirmation IN ('attendance', 'animal_delivered', 'animal_received')),
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    notes text,
    UNIQUE (appointment_id, party, confirmation)
);

CREATE TABLE appointment_otps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_id uuid NOT NULL REFERENCES appointments(id) ON DELETE CASCADE,
    code_hash bytea NOT NULL CHECK (octet_length(code_hash) >= 32),
    expires_at timestamptz NOT NULL,
    max_attempts smallint NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 20),
    failed_attempts smallint NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    used_at timestamptz,
    revoked_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    CHECK (failed_attempts <= max_attempts),
    CHECK (used_at IS NULL OR revoked_at IS NULL)
);

CREATE UNIQUE INDEX appointment_otps_active_uidx
    ON appointment_otps (appointment_id)
    WHERE used_at IS NULL AND revoked_at IS NULL;

CREATE TABLE post_adoption_followups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES adoption_applications(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    occurred_at timestamptz NOT NULL,
    contact_method text NOT NULL CHECK (contact_method IN ('in_app', 'phone', 'email', 'visit')),
    notes text NOT NULL CHECK (length(btrim(notes)) > 0),
    welfare_status text NOT NULL CHECK (welfare_status IN ('good', 'needs_attention', 'urgent')),
    recorded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL,
    animal_id uuid REFERENCES animals(id) ON DELETE SET NULL,
    application_id uuid REFERENCES adoption_applications(id) ON DELETE SET NULL,
    conversation_id uuid REFERENCES conversations(id) ON DELETE SET NULL,
    message_id uuid REFERENCES messages(id) ON DELETE SET NULL,
    category text NOT NULL CHECK (category IN ('payment_request', 'fraud', 'harassment', 'inappropriate_content', 'off_platform_contact', 'animal_welfare', 'other')),
    description text NOT NULL CHECK (length(btrim(description)) BETWEEN 10 AND 5000),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'triaged', 'investigating', 'resolved', 'dismissed')),
    priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    assigned_to uuid REFERENCES users(id) ON DELETE SET NULL,
    resolution text,
    resolved_by uuid REFERENCES users(id) ON DELETE SET NULL,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(organization_id, animal_id, application_id, conversation_id, message_id) >= 1),
    CHECK ((status IN ('resolved', 'dismissed')) = (resolved_at IS NOT NULL)),
    CHECK (status NOT IN ('resolved', 'dismissed') OR resolved_by IS NOT NULL)
);

CREATE TABLE audit_log (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL,
    action text NOT NULL CHECK (length(btrim(action)) > 0),
    entity_type text NOT NULL CHECK (entity_type ~ '^[a-z][a-z_]*$'),
    entity_id uuid,
    old_values jsonb,
    new_values jsonb,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    request_id uuid,
    ip_address inet,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_active_email_idx ON users (email) WHERE deleted_at IS NULL AND deactivated_at IS NULL;
CREATE INDEX refresh_sessions_user_active_idx ON refresh_sessions (user_id, expires_at DESC) WHERE revoked_at IS NULL;
CREATE INDEX password_reset_tokens_user_active_idx ON password_reset_tokens (user_id, expires_at DESC) WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE INDEX organizations_verified_location_idx ON organizations (state_code, city) WHERE verification_status = 'verified' AND deactivated_at IS NULL;
CREATE INDEX organization_members_user_active_idx ON organization_members (user_id, organization_id) WHERE status = 'active';
CREATE INDEX animals_discovery_idx ON animals (status, state_code, city, published_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX animals_organization_idx ON animals (responsible_organization_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX animals_submitter_idx ON animals (submitted_by, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX animal_images_gallery_idx ON animal_images (animal_id, position, created_at) WHERE removed_at IS NULL;
CREATE INDEX animal_preferences_user_kind_idx ON animal_preferences (user_id, preference, updated_at DESC);
CREATE INDEX adoption_applications_org_status_idx ON adoption_applications (organization_id, status, created_at DESC);
CREATE INDEX adoption_applications_adopter_idx ON adoption_applications (adopter_id, created_at DESC);
CREATE INDEX application_status_history_application_idx ON application_status_history (application_id, created_at DESC);
CREATE INDEX conversations_org_status_idx ON conversations (organization_id, status, updated_at DESC);
CREATE INDEX conversations_counterparty_idx ON conversations (counterparty_user_id, updated_at DESC);
CREATE INDEX messages_conversation_idx ON messages (conversation_id, created_at DESC, id);
CREATE INDEX appointments_upcoming_idx ON appointments (organization_id, starts_at) WHERE status IN ('proposed', 'scheduled');
CREATE INDEX post_adoption_followups_application_idx ON post_adoption_followups (application_id, occurred_at DESC);
CREATE INDEX reports_queue_idx ON reports (status, priority, created_at) WHERE status NOT IN ('resolved', 'dismissed');
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_user_id, created_at DESC);

CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER organizations_set_updated_at BEFORE UPDATE ON organizations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER organization_members_set_updated_at BEFORE UPDATE ON organization_members FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER animals_set_updated_at BEFORE UPDATE ON animals FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER animal_preferences_set_updated_at BEFORE UPDATE ON animal_preferences FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER adoption_applications_set_updated_at BEFORE UPDATE ON adoption_applications FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER application_screenings_set_updated_at BEFORE UPDATE ON application_screenings FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER conversations_set_updated_at BEFORE UPDATE ON conversations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER appointments_set_updated_at BEFORE UPDATE ON appointments FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER reports_set_updated_at BEFORE UPDATE ON reports FOR EACH ROW EXECUTE FUNCTION set_updated_at();
