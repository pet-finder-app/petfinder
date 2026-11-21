ALTER TABLE followup_tasks DROP CONSTRAINT followup_tasks_cancellation_check;
UPDATE followup_tasks SET status = 'closed', closed_at = COALESCE(closed_at, now()) WHERE status = 'cancelled';
ALTER TABLE followup_tasks DROP COLUMN cancellation_reason;
ALTER TABLE followup_tasks DROP CONSTRAINT followup_tasks_status_check;
ALTER TABLE followup_tasks ADD CONSTRAINT followup_tasks_status_check
    CHECK (status IN ('scheduled', 'submitted', 'changes_requested', 'closed'));
