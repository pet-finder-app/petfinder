CREATE TABLE followup_tasks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),application_id uuid NOT NULL REFERENCES adoption_applications(id),organization_id uuid NOT NULL REFERENCES organizations(id),
 due_at timestamptz NOT NULL,instructions text NOT NULL,status text NOT NULL DEFAULT 'scheduled' CHECK(status IN ('scheduled','submitted','changes_requested','closed')),
 created_by uuid NOT NULL REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now(),closed_at timestamptz
);
CREATE TABLE followup_reports (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),followup_id uuid NOT NULL REFERENCES followup_tasks(id),submitted_by uuid NOT NULL REFERENCES users(id),
 notes text NOT NULL,welfare_status text NOT NULL CHECK(welfare_status IN ('good','needs_attention','urgent')),media_ids uuid[] NOT NULL DEFAULT '{}',submitted_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE followup_reviews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),followup_id uuid NOT NULL REFERENCES followup_tasks(id),reviewed_by uuid NOT NULL REFERENCES users(id),
 decision text NOT NULL CHECK(decision IN ('closed','changes_requested')),reason text NOT NULL,reviewed_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX notifications_followup_due_idx ON notifications(user_id,subject_id) WHERE kind='followup_due';
CREATE FUNCTION notify_followup() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO notifications(user_id,kind,subject_id) SELECT adopter_id,'followup',NEW.id FROM adoption_applications WHERE id=NEW.application_id;
 INSERT INTO notifications(user_id,kind,subject_id) SELECT user_id,'followup',NEW.id FROM organization_members WHERE organization_id=NEW.organization_id AND status='active' AND can_manage_applications;
 RETURN NEW;END $$;
CREATE TRIGGER followup_notify AFTER INSERT OR UPDATE OF status ON followup_tasks FOR EACH ROW EXECUTE FUNCTION notify_followup();
