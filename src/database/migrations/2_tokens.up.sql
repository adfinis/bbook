CREATE TABLE
    tokens (
        id UUID PRIMARY KEY DEFAULT uuidv7 (),
        user_sub TEXT NOT NULL,
        name TEXT NOT NULL DEFAULT '',
        expires_at TIMESTAMPTZ,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );

CREATE INDEX tokens_user_sub_idx ON tokens (user_sub);

CREATE TABLE
    user_offline_tokens (
        user_sub TEXT PRIMARY KEY,
        offline_token BYTEA NOT NULL,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now ()
    );