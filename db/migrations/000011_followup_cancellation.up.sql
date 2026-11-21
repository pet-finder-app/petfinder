-- Organizations may reschedule or cancel a follow-up before it is closed.
ALTER TABLE followup_tasks DROP CONSTRAINT followup_tasks_status_check;
ALTER TABLE followup_tasks ADD CONSTRAINT followup_tasks_status_check
    CHECK (status IN ('scheduled', 'submitted', 'changes_requested', 'closed', 'cancelled'));
ALTER TABLE followup_tasks ADD COLUMN cancellation_reason text;
ALTER TABLE followup_tasks ADD CONSTRAINT followup_tasks_cancellation_check
    CHECK ((status = 'cancelled') = (cancellation_reason IS NOT NULL));
