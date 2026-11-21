CREATE TABLE surrenders (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),requester_id uuid NOT NULL REFERENCES users(id),organization_id uuid NOT NULL REFERENCES organizations(id),
 kind text NOT NULL CHECK(kind IN ('intake','return')),animal_id uuid REFERENCES animals(id),application_id uuid REFERENCES adoption_applications(id),
 animal_snapshot jsonb NOT NULL,reason text NOT NULL,documents uuid[] NOT NULL DEFAULT '{}',
 status text NOT NULL DEFAULT 'submitted' CHECK(status IN ('submitted','needs_information','accepted','rejected','cancelled','completed')),
 review_reason text,reviewed_by uuid REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 handover_code_hash text,code_expires_at timestamptz,code_failed_attempts integer NOT NULL DEFAULT 0,
 delivered_at timestamptz,received_at timestamptz,completed_at timestamptz,
 CHECK(kind<>'return' OR application_id IS NOT NULL)
);
CREATE UNIQUE INDEX surrenders_active_animal_idx ON surrenders(animal_id) WHERE status IN ('submitted','needs_information','accepted');
CREATE TABLE surrender_history(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,surrender_id uuid NOT NULL REFERENCES surrenders(id),actor_id uuid NOT NULL REFERENCES users(id),action text NOT NULL,notes text NOT NULL,snapshot jsonb,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE agreement_templates (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),organization_id uuid NOT NULL REFERENCES organizations(id),kind text NOT NULL CHECK(kind IN ('adoption','surrender')),
 version integer NOT NULL CHECK(version>0),body text NOT NULL CHECK(length(btrim(body))>0),created_by uuid NOT NULL REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,kind,version)
);
CREATE TABLE agreements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),application_id uuid UNIQUE REFERENCES adoption_applications(id),surrender_id uuid UNIQUE REFERENCES surrenders(id),
 organization_id uuid NOT NULL REFERENCES organizations(id),template_id uuid NOT NULL REFERENCES agreement_templates(id),
 user_id uuid NOT NULL REFERENCES users(id),representative_id uuid NOT NULL REFERENCES users(id),
 body text NOT NULL,sha256 text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),CHECK(num_nonnulls(application_id,surrender_id)=1),CHECK(user_id<>representative_id)
);
CREATE TABLE agreement_codes (
 agreement_id uuid NOT NULL REFERENCES agreements(id),signer_id uuid NOT NULL REFERENCES users(id),code_hash text NOT NULL,expires_at timestamptz NOT NULL,
 failed_attempts integer NOT NULL DEFAULT 0 CHECK(failed_attempts BETWEEN 0 AND 5),used_at timestamptz,PRIMARY KEY(agreement_id,signer_id)
);
CREATE TABLE agreement_signatures (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),agreement_id uuid NOT NULL REFERENCES agreements(id),signer_id uuid NOT NULL REFERENCES users(id),
 sha256 text NOT NULL,ip_address text NOT NULL,user_agent text NOT NULL,signed_at timestamptz NOT NULL DEFAULT now(),UNIQUE(agreement_id,signer_id)
);
