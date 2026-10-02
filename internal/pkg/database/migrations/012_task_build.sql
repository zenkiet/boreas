-- The latest report a CI pipeline sent. A report is not a task edit, so it leaves updated_at alone.
ALTER TABLE tasks ADD COLUMN build JSONB NOT NULL DEFAULT '{}';
DROP TRIGGER tasks_set_updated_at ON tasks;
CREATE TRIGGER tasks_set_updated_at
    BEFORE UPDATE ON tasks
    FOR EACH ROW WHEN (OLD.build IS NOT DISTINCT FROM NEW.build) EXECUTE FUNCTION set_updated_at();

ALTER TABLE notifications DROP CONSTRAINT notifications_type_check,
    ADD CONSTRAINT notifications_type_check CHECK (type IN
        ('deployed', 'deploy_failed', 'status_changed', 'task_created', 'task_assigned', 'build_failed'));
