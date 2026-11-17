-- Why a spec's last step broke off: someone pressed Stop, or it failed. Empty while a step runs, after one
-- succeeds, or when the server went down mid-step.
ALTER TABLE bora_specs
    ADD COLUMN stop       TEXT NOT NULL DEFAULT '' CHECK (stop IN ('', 'stopped', 'failed')),
    ADD COLUMN stopped_by UUID REFERENCES users(id) ON DELETE SET NULL;
