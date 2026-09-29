-- Clients route and filter on the event, which the title only spells out for people.
ALTER TABLE notifications ADD COLUMN type TEXT;
UPDATE notifications SET type = CASE
    WHEN status = 'success' THEN 'deployed'
    WHEN status = 'failure' THEN 'deploy_failed'
    WHEN title LIKE '%Task Created%' THEN 'task_created'
    WHEN title LIKE '%Task Assigned%' THEN 'task_assigned'
    ELSE 'status_changed' END;
ALTER TABLE notifications ALTER COLUMN type SET NOT NULL,
    ADD CONSTRAINT notifications_type_check
        CHECK (type IN ('deployed', 'deploy_failed', 'status_changed', 'task_created', 'task_assigned'));
