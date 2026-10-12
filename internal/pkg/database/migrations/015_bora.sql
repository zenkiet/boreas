CREATE EXTENSION IF NOT EXISTS unaccent;
-- unaccent() is STABLE, and a generated column needs IMMUTABLE.
CREATE FUNCTION f_unaccent(text) RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT public.unaccent('public.unaccent', $1) $$;

DROP TABLE chats;

CREATE TABLE bora_conversations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID        REFERENCES projects(id) ON DELETE CASCADE, -- NULL: every project
    about      JSONB       NOT NULL,
    projects   UUID[]      NOT NULL DEFAULT '{}',
    title      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON bora_conversations (user_id, updated_at DESC, id DESC);

CREATE TABLE bora_messages (
    conversation_id UUID        NOT NULL REFERENCES bora_conversations(id) ON DELETE CASCADE,
    index           INT         NOT NULL,
    role            TEXT        NOT NULL CHECK (role IN ('user', 'assistant')),
    content         TEXT        NOT NULL,
    say             TEXT        NOT NULL DEFAULT '',
    actions         JSONB       NOT NULL DEFAULT '[]',
    choices         JSONB       NOT NULL DEFAULT '[]',
    sources         JSONB       NOT NULL DEFAULT '[]',
    ran             JSONB       NOT NULL DEFAULT '[]',
    at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    search          TSVECTOR    GENERATED ALWAYS AS (to_tsvector('simple', f_unaccent(content))) STORED,
    PRIMARY KEY (conversation_id, index)
);
CREATE INDEX ON bora_messages USING GIN (search);
