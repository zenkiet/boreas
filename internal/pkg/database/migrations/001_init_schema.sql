CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

--- AUTH ---
CREATE TABLE IF NOT EXISTS users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    disabled_at   TIMESTAMPTZ, -- NULL = working
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Unique is not case sensitive: 'Admin' and 'admin' cannot be two accounts.
CREATE UNIQUE INDEX IF NOT EXISTS users_username_key ON users (lower(username));
CREATE UNIQUE INDEX IF NOT EXISTS users_email_key    ON users (lower(email));

DROP TRIGGER IF EXISTS users_set_updated_at ON users;
CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Save only the hash (SHA-256) of the token; The original token returns exactly once at creation.
CREATE TABLE IF NOT EXISTS auth_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ, -- NULL = available
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_tokens_user_id_idx ON auth_tokens (user_id);

--- REGISTER CREDENTIALS---
CREATE TABLE IF NOT EXISTS registry_credentials (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL UNIQUE,
    registry   TEXT        NOT NULL DEFAULT 'ghcr' CHECK (registry IN ('ghcr', 'dockerhub')),
    username   TEXT        NOT NULL,
    token      TEXT        NOT NULL,
    created_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

--- PROJECTS ---
-- slug is the first path segment of a task URL, so it may not collide with the
-- application's own top-level routes.
CREATE TABLE IF NOT EXISTS projects (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       TEXT        NOT NULL UNIQUE
               CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$')
               CHECK (slug NOT IN ('api', 'health', 'metrics', 'static', 'admin')),
    name       TEXT        NOT NULL,
    registry_credential_id UUID REFERENCES registry_credentials(id) ON DELETE SET NULL, -- NULL for image public
    created_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS projects_registry_credential_id_idx ON projects (registry_credential_id);
DROP TRIGGER IF EXISTS projects_set_updated_at ON projects;
CREATE TRIGGER projects_set_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();


CREATE TABLE IF NOT EXISTS project_members (
    project_id UUID        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    role       TEXT        NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX IF NOT EXISTS project_members_user_id_idx ON project_members (user_id);

--- TASKS ---
CREATE TABLE IF NOT EXISTS tasks (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       UUID        NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    name             TEXT        NOT NULL
                     CHECK (name ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$'),
    description      TEXT        NOT NULL DEFAULT '',
    image            TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'unknown'
                     CHECK (status IN ('creating', 'starting', 'running', 'stopped', 'error', 'unknown')),
    port             INTEGER     NOT NULL DEFAULT 80
                     CHECK (port BETWEEN 1 AND 65535),
    container_id     TEXT        NOT NULL DEFAULT '',
    container_ip     TEXT        NOT NULL DEFAULT '',
    labels           JSONB       NOT NULL DEFAULT '{}',
    env              JSONB       NOT NULL DEFAULT '{}',
    pending_recreate BOOLEAN     NOT NULL DEFAULT FALSE,
    error            TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);
DROP TRIGGER IF EXISTS tasks_set_updated_at ON tasks;
CREATE TRIGGER tasks_set_updated_at
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

--- METRICS ---
-- memory_bytes here is an observed sample, not a limit.
CREATE TABLE IF NOT EXISTS task_metrics (
    task_id          UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    observed_at      TIMESTAMPTZ NOT NULL,
    cpu_percent      REAL        NOT NULL DEFAULT 0,
    memory_bytes     BIGINT      NOT NULL DEFAULT 0,
    network_rx_bytes BIGINT      NOT NULL DEFAULT 0,
    network_tx_bytes BIGINT      NOT NULL DEFAULT 0,
    PRIMARY KEY (task_id, observed_at)
);
CREATE INDEX IF NOT EXISTS task_metrics_observed_at_brin ON task_metrics USING brin (observed_at);
