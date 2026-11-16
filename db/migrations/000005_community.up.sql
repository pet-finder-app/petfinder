CREATE TABLE media_uploads (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner_id uuid NOT NULL REFERENCES users(id),
 filename text NOT NULL,content_type text NOT NULL CHECK(content_type IN ('image/jpeg','image/png','image/webp','application/pdf')),
 byte_size integer NOT NULL CHECK(byte_size BETWEEN 1 AND 10485760),
 purpose text NOT NULL CHECK(purpose IN ('avatar','animal','post','document','followup','surrender')),
 content bytea,created_at timestamptz NOT NULL DEFAULT now(),uploaded_at timestamptz,
 CHECK(content IS NULL OR octet_length(content)=byte_size)
);
CREATE TABLE posts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),organization_id uuid NOT NULL REFERENCES organizations(id),
 author_id uuid NOT NULL REFERENCES users(id),body text NOT NULL CHECK(length(btrim(body)) BETWEEN 1 AND 10000),
 media_ids uuid[] NOT NULL DEFAULT '{}',tags text[] NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),deleted_at timestamptz
);
CREATE INDEX posts_organization_created_idx ON posts(organization_id,created_at DESC,id) WHERE deleted_at IS NULL;
CREATE TABLE post_likes (post_id uuid NOT NULL REFERENCES posts(id),user_id uuid NOT NULL REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(post_id,user_id));
CREATE TABLE organization_followers (organization_id uuid NOT NULL REFERENCES organizations(id),user_id uuid NOT NULL REFERENCES users(id),created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(organization_id,user_id));
CREATE TABLE notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES users(id),kind text NOT NULL,subject_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),read_at timestamptz
);
CREATE INDEX notifications_user_created_idx ON notifications(user_id,created_at DESC,id);
CREATE TABLE conversation_reads (
 conversation_id uuid NOT NULL REFERENCES conversations(id),user_id uuid NOT NULL REFERENCES users(id),
 message_id uuid NOT NULL REFERENCES messages(id),message_created_at timestamptz NOT NULL,read_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(conversation_id,user_id)
);
ALTER TABLE conversations DROP CONSTRAINT conversations_kind_check;
ALTER TABLE conversations ADD CONSTRAINT conversations_kind_check CHECK(kind IN ('adopter_organization','owner_organization','interested_organization'));
CREATE UNIQUE INDEX conversations_interest_idx ON conversations(animal_id,counterparty_user_id,organization_id) WHERE kind='interested_organization';
ALTER TABLE reports ADD COLUMN post_id uuid REFERENCES posts(id);
ALTER TABLE reports DROP CONSTRAINT reports_check;
ALTER TABLE reports ADD CONSTRAINT reports_subject_check CHECK(num_nonnulls(organization_id,animal_id,application_id,conversation_id,message_id,post_id)>=1);
CREATE FUNCTION notify_message_participants() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notifications(user_id,kind,subject_id)
 SELECT recipient,'message',NEW.conversation_id FROM (
 SELECT c.counterparty_user_id recipient FROM conversations c WHERE c.id=NEW.conversation_id
 UNION SELECT om.user_id FROM conversations c JOIN organization_members om ON om.organization_id=c.organization_id AND om.status='active' WHERE c.id=NEW.conversation_id
 ) recipients WHERE recipient<>NEW.sender_id;
 RETURN NEW;
END $$;
CREATE TRIGGER messages_notify AFTER INSERT ON messages FOR EACH ROW EXECUTE FUNCTION notify_message_participants();
CREATE FUNCTION notify_application_participants() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notifications(user_id,kind,subject_id)
 SELECT recipient,'application',NEW.id FROM (
 SELECT NEW.adopter_id recipient
 UNION SELECT user_id FROM organization_members WHERE organization_id=NEW.organization_id AND status='active'
 ) recipients;
 RETURN NEW;
END $$;
CREATE TRIGGER application_notify AFTER INSERT OR UPDATE OF status ON adoption_applications FOR EACH ROW EXECUTE FUNCTION notify_application_participants();
CREATE FUNCTION notify_new_post() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO notifications(user_id,kind,subject_id) SELECT user_id,'post',NEW.id FROM organization_followers WHERE organization_id=NEW.organization_id AND user_id<>NEW.author_id;
 RETURN NEW;
END $$;
CREATE TRIGGER post_notify AFTER INSERT ON posts FOR EACH ROW EXECUTE FUNCTION notify_new_post();

CREATE TABLE animal_preference_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,animal_id uuid NOT NULL REFERENCES animals(id),user_id uuid NOT NULL REFERENCES users(id),preference text CHECK(preference IN ('liked','not_interested')),created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX preference_events_animal_created_idx ON animal_preference_events(animal_id,created_at);
CREATE FUNCTION record_preference_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='DELETE' THEN
 INSERT INTO animal_preference_events(animal_id,user_id,preference) VALUES(OLD.animal_id,OLD.user_id,NULL);
 UPDATE conversations SET status='closed',closed_at=now(),closed_by=OLD.user_id WHERE animal_id=OLD.animal_id AND counterparty_user_id=OLD.user_id AND kind='interested_organization' AND status='open';
 RETURN OLD;
 END IF;
 IF TG_OP='INSERT' OR NEW.preference IS DISTINCT FROM OLD.preference THEN
 INSERT INTO animal_preference_events(animal_id,user_id,preference) VALUES(NEW.animal_id,NEW.user_id,NEW.preference);
 IF NEW.preference='not_interested' THEN UPDATE conversations SET status='closed',closed_at=now(),closed_by=NEW.user_id WHERE animal_id=NEW.animal_id AND counterparty_user_id=NEW.user_id AND kind='interested_organization' AND status='open'; END IF;
 END IF;
 RETURN NEW;END $$;
CREATE TRIGGER preference_event AFTER INSERT OR UPDATE OR DELETE ON animal_preferences FOR EACH ROW EXECUTE FUNCTION record_preference_event();
CREATE FUNCTION close_contact_on_revocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.personal_profile->>'allow_organization_contact' IS DISTINCT FROM 'true' THEN
 UPDATE conversations SET status='closed',closed_at=now(),closed_by=NEW.id WHERE counterparty_user_id=NEW.id AND kind='interested_organization' AND status='open';
 END IF;RETURN NEW;END $$;
CREATE TRIGGER contact_revocation AFTER UPDATE OF personal_profile ON users FOR EACH ROW EXECUTE FUNCTION close_contact_on_revocation();
