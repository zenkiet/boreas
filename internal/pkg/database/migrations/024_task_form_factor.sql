-- The phone frame follows each task; the project's value is only what new tasks start with.
ALTER TABLE projects RENAME COLUMN form_factor TO default_form_factor;

ALTER TABLE tasks
    ADD COLUMN form_factor TEXT NOT NULL DEFAULT 'web'
        CHECK (form_factor IN ('web', 'mobile'));

-- Existing tasks keep the frame their project gave them; copying it over is not a task edit.
ALTER TABLE tasks DISABLE TRIGGER tasks_set_updated_at;
UPDATE tasks SET form_factor = projects.default_form_factor
FROM projects WHERE projects.id = tasks.project_id;
ALTER TABLE tasks ENABLE TRIGGER tasks_set_updated_at;
