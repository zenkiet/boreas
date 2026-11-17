-- What a sign-in runs on, as its User-Agent names it, and when any token last answered a request (to the minute).
ALTER TABLE auth_tokens
    ADD COLUMN device TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_used_at TIMESTAMPTZ;
