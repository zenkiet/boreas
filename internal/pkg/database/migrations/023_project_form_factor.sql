-- A mobile project's tasks are Capacitor apps served as websites; desktops show them in a phone frame.
ALTER TABLE projects
    ADD COLUMN form_factor TEXT NOT NULL DEFAULT 'web'
        CHECK (form_factor IN ('web', 'mobile'));
