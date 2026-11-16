ALTER TABLE users ADD COLUMN personal_profile jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(personal_profile)='object');
ALTER TABLE users ADD COLUMN personal_verification_status text NOT NULL DEFAULT 'unverified' CHECK(personal_verification_status IN ('unverified','pending','verified','rejected'));
ALTER TABLE organizations ADD COLUMN public_profile jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(public_profile)='object');
ALTER TABLE organizations ADD COLUMN private_profile jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(private_profile)='object');
CREATE TABLE verification_submissions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id uuid REFERENCES users(id), organization_id uuid REFERENCES organizations(id),
 target_snapshot jsonb NOT NULL DEFAULT '{}',
 evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object'),
 status text NOT NULL DEFAULT 'submitted' CHECK(status IN ('submitted','under_review','approved','rejected')),
 submitted_by uuid NOT NULL REFERENCES users(id), reviewed_by uuid REFERENCES users(id), reason text,
 created_at timestamptz NOT NULL DEFAULT now(), reviewed_at timestamptz,
 CHECK(num_nonnulls(user_id,organization_id)=1),
 CHECK((status IN ('approved','rejected'))=(reviewed_at IS NOT NULL))
);
CREATE UNIQUE INDEX verification_user_pending ON verification_submissions(user_id) WHERE status IN ('submitted','under_review');
CREATE UNIQUE INDEX verification_org_pending ON verification_submissions(organization_id) WHERE status IN ('submitted','under_review');
