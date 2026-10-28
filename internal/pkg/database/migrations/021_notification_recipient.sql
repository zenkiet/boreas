-- A notification with a recipient reaches only them, in the feed and by push; without one, everyone who sees its task.
ALTER TABLE notifications ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;
