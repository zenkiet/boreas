CREATE INDEX IF NOT EXISTS notifications_last_deploy_idx
    ON notifications (project_id, task_name, created_at DESC)
    WHERE type IN ('deployed', 'deploy_failed');
