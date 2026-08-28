-- Lifecycle events (task created, assigned, dev status changed) are neither a
-- success nor a failure.
ALTER TABLE notifications DROP CONSTRAINT notifications_status_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_status_check CHECK (status IN ('success', 'failure', 'info'));

-- Seen is per user: one member reading a notification must not clear it for
-- the rest of the project.
CREATE TABLE IF NOT EXISTS notification_seen (
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (notification_id, user_id)
);
