ALTER TABLE notifications DROP CONSTRAINT notifications_type_check,
    ADD CONSTRAINT notifications_type_check CHECK (type IN
        ('deployed', 'deploy_failed', 'status_changed', 'task_created', 'task_assigned', 'build_failed',
         'container_down', 'spec_asking', 'spec_confirming', 'spec_ready', 'spec_failed', 'support_request'));
