-- Who did it, when a signed-in person did: the feed names them and their own devices get no push.
-- A planning notification also names its spec, which it opens.
ALTER TABLE notifications
    ADD COLUMN actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN spec_id  UUID REFERENCES bora_specs(id) ON DELETE CASCADE;
ALTER TABLE notifications DROP CONSTRAINT notifications_type_check,
    ADD CONSTRAINT notifications_type_check CHECK (type IN
        ('deployed', 'deploy_failed', 'status_changed', 'task_created', 'task_assigned', 'build_failed',
         'container_down', 'spec_asking', 'spec_confirming', 'spec_ready', 'spec_failed'));
