-- important: failures and what is addressed to the device's user; all: everything the feed shows them.
ALTER TABLE push_subscriptions
    ADD COLUMN events TEXT NOT NULL DEFAULT 'important' CHECK (events IN ('important', 'all'));
