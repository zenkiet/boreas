ALTER TABLE auth_tokens
    ADD COLUMN name TEXT NOT NULL DEFAULT '',
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'session',
    ADD COLUMN valid_from TIMESTAMPTZ;

UPDATE auth_tokens
SET valid_from = created_at
WHERE valid_from IS NULL;

ALTER TABLE auth_tokens
    ALTER COLUMN valid_from SET NOT NULL,
    ADD CONSTRAINT auth_tokens_kind_check
        CHECK (kind IN ('session', 'api')),
    ADD CONSTRAINT auth_tokens_valid_window_check
        CHECK (valid_from < expires_at),
    ADD CONSTRAINT auth_tokens_api_name_check
        CHECK (
            (kind = 'session' AND name = '') OR
            (kind = 'api' AND char_length(btrim(name)) BETWEEN 1 AND 100)
        ),
    ADD CONSTRAINT auth_tokens_api_lifetime_check
        CHECK (kind <> 'api' OR expires_at - valid_from <= interval '90 days');
