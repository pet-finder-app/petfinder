CREATE TABLE organization_payment_details (
 organization_id uuid PRIMARY KEY REFERENCES organizations(id),method text NOT NULL CHECK(method IN ('pix','bank_transfer')),
 instructions text NOT NULL,pix_key text,bank_name text,bank_branch text,bank_account text,account_holder text NOT NULL,
 published boolean NOT NULL DEFAULT false,updated_by uuid NOT NULL REFERENCES users(id),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE donations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),organization_id uuid NOT NULL REFERENCES organizations(id),donor_id uuid NOT NULL REFERENCES users(id),
 amount_cents bigint NOT NULL CHECK(amount_cents BETWEEN 1 AND 1000000000),currency text NOT NULL DEFAULT 'BRL' CHECK(currency='BRL'),
 transferred_at timestamptz NOT NULL,receipt_media_id uuid REFERENCES media_uploads(id),anonymous boolean NOT NULL DEFAULT true,
 note text NOT NULL DEFAULT '',status text NOT NULL DEFAULT 'reported' CHECK(status IN ('reported','confirmed','not_found')),
 payment_snapshot jsonb NOT NULL,decided_by uuid REFERENCES users(id),decided_at timestamptz,decision_reason text,created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX donations_org_created_idx ON donations(organization_id,created_at DESC,id);
CREATE INDEX donations_donor_created_idx ON donations(donor_id,created_at DESC,id);
CREATE TABLE donation_history(id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,donation_id uuid NOT NULL REFERENCES donations(id),status text NOT NULL,reason text NOT NULL,actor_id uuid NOT NULL REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now());
CREATE FUNCTION notify_donation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO notifications(user_id,kind,subject_id) SELECT NEW.donor_id,'donation',NEW.id
 UNION SELECT user_id,'donation',NEW.id FROM organization_members WHERE organization_id=NEW.organization_id AND status='active' AND can_manage_members;
 RETURN NEW;END $$;
CREATE TRIGGER donation_notify AFTER INSERT OR UPDATE OF status ON donations FOR EACH ROW EXECUTE FUNCTION notify_donation();
