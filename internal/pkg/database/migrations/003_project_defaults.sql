ALTER TABLE projects
    ADD COLUMN default_image TEXT    NOT NULL DEFAULT '',
    ADD COLUMN default_port  INTEGER NOT NULL DEFAULT 80
        CHECK (default_port BETWEEN 1 AND 65535),
    ADD COLUMN default_env   JSONB   NOT NULL DEFAULT '{}';
