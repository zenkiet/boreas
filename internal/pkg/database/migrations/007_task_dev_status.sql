-- Development progress is independent of the container: a task can run while its
-- code is not yet ready for QA.
ALTER TABLE tasks
    ADD COLUMN dev_status TEXT NOT NULL DEFAULT 'in_progress'
        CHECK (dev_status IN ('in_progress', 'blocked', 'ready'));
