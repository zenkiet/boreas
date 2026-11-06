CREATE UNIQUE INDEX IF NOT EXISTS tasks_project_lower_name_key ON tasks (project_id, lower(name));
