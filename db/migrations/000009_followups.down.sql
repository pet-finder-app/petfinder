DROP TRIGGER followup_notify ON followup_tasks;
DROP FUNCTION notify_followup();
DROP INDEX notifications_followup_due_idx;
DROP TABLE followup_reviews,followup_reports,followup_tasks;
