CREATE TABLE contacts (
    zoho_id       TEXT PRIMARY KEY,
    first_name    TEXT NOT NULL,
    last_name     TEXT NOT NULL,
    organization  TEXT NOT NULL,
    email         TEXT NOT NULL,
    phone         TEXT NOT NULL,
    mobile        TEXT NOT NULL,
    street        TEXT NOT NULL,
    city          TEXT NOT NULL,
    postal_code   TEXT NOT NULL,
    status        TEXT NOT NULL,
    modified_time TIMESTAMPTZ NOT NULL,
    raw           JSONB NOT NULL,
    synced_at     TIMESTAMPTZ NOT NULL
);
