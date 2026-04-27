-- 0003_sessions.up.sql
-- Canonical schema for alexedwards/scs/pgxstore.
-- DO NOT modify column names or types — pgxstore queries this table verbatim.
-- See pkg.go.dev/github.com/alexedwards/scs/pgxstore for the reference.
CREATE TABLE sessions (
    token   TEXT        PRIMARY KEY,
    data    BYTEA       NOT NULL,
    expiry  TIMESTAMPTZ NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions (expiry);
