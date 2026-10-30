-- A project's code is now the Sourcebot search context named after it.
ALTER TABLE projects DROP COLUMN repositories;

-- The commit a task's image was built from, and the one the deploy before ran, as its pipeline reported them.
ALTER TABLE tasks ADD COLUMN revision JSONB NOT NULL DEFAULT '{}';
