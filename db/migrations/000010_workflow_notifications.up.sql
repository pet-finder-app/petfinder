CREATE FUNCTION notify_surrender() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO notifications(user_id,kind,subject_id) SELECT NEW.requester_id,'surrender',NEW.id
 UNION SELECT user_id,'surrender',NEW.id FROM organization_members WHERE organization_id=NEW.organization_id AND status='active' AND can_manage_applications;
 RETURN NEW;END $$;
CREATE TRIGGER surrender_notify AFTER INSERT OR UPDATE OF status ON surrenders FOR EACH ROW EXECUTE FUNCTION notify_surrender();
CREATE FUNCTION notify_appointment() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO notifications(user_id,kind,subject_id) SELECT adopter_id,'appointment',NEW.id FROM adoption_applications WHERE id=NEW.application_id
 UNION SELECT user_id,'appointment',NEW.id FROM organization_members WHERE organization_id=NEW.organization_id AND status='active' AND can_manage_applications;
 RETURN NEW;END $$;
CREATE TRIGGER appointment_notify AFTER INSERT OR UPDATE OF status,starts_at ON appointments FOR EACH ROW EXECUTE FUNCTION notify_appointment();
CREATE FUNCTION notify_verification() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO notifications(user_id,kind,subject_id) VALUES(NEW.submitted_by,'verification',NEW.id);
 RETURN NEW;END $$;
CREATE TRIGGER verification_notify AFTER UPDATE OF status ON verification_submissions FOR EACH ROW EXECUTE FUNCTION notify_verification();
