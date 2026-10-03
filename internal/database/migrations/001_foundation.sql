CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    login TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('engineer', 'coordinator')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE resources (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX resources_code_case_insensitive_uq ON resources ((lower(code)));

CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    account_id UUID REFERENCES accounts(id),
    csrf_token BYTEA NOT NULL CHECK (octet_length(csrf_token) = 32),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL CHECK (expires_at > created_at)
);

CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

GRANT SELECT ON accounts, resources TO labreserve_app;
GRANT SELECT, INSERT, DELETE ON sessions TO labreserve_app;
