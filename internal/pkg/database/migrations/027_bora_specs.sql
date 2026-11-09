CREATE TABLE bora_specs (
    id         UUID        PRIMARY KEY,
    key        TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    projects   UUID[]      NOT NULL,
    slugs      TEXT[]      NOT NULL,
    state      TEXT        NOT NULL CHECK (state IN ('researching', 'asking', 'confirming', 'writing', 'ready')),
    body       JSONB       NOT NULL,
    created_by UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON bora_specs (lower(key));
