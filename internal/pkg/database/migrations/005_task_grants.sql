ALTER TABLE project_members DROP CONSTRAINT IF EXISTS project_members_role_check;
ALTER TABLE project_members ADD CONSTRAINT project_members_role_check
    CHECK (role IN ('viewer', 'operator', 'member', 'owner'));

-- Grants die with the task, so access can never outlive what it points at.
-- 'owner' is absent on purpose: it only means anything at project scope.
CREATE TABLE IF NOT EXISTS task_grants (
    task_id    UUID        NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL CHECK (role IN ('viewer', 'operator', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, user_id)
);
CREATE INDEX IF NOT EXISTS task_grants_user_id_idx ON task_grants (user_id);
