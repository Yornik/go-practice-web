-- The initial schema. Times are UTC RFC 3339 text with milliseconds and a
-- fixed width (2026-10-17T09:12:33.000Z), so that text order is time order.

CREATE TABLE users (
    id                INTEGER PRIMARY KEY,
    username          TEXT    NOT NULL UNIQUE
                              CHECK (length(username) BETWEEN 3 AND 32 AND username NOT GLOB '*[^a-z0-9_-]*'),
    display_name      TEXT    NOT NULL,
    email             TEXT    UNIQUE COLLATE NOCASE,
    email_verified_at TEXT,
    is_admin          INTEGER NOT NULL DEFAULT 0,
    disabled_at       TEXT,
    webauthn_handle   BLOB    NOT NULL UNIQUE,
    created_at        TEXT    NOT NULL
);

CREATE TABLE credentials (
    id               BLOB    PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    public_key       BLOB    NOT NULL,
    attestation_type TEXT    NOT NULL DEFAULT '',
    aaguid           BLOB,
    sign_count       INTEGER NOT NULL DEFAULT 0,
    clone_warning    INTEGER NOT NULL DEFAULT 0,
    transports       TEXT    NOT NULL DEFAULT '',
    attachment       TEXT    NOT NULL DEFAULT '',
    user_present     INTEGER NOT NULL DEFAULT 0,
    user_verified    INTEGER NOT NULL DEFAULT 0,
    backup_eligible  INTEGER NOT NULL DEFAULT 0,
    backup_state     INTEGER NOT NULL DEFAULT 0,
    name             TEXT    NOT NULL DEFAULT '',
    created_at       TEXT    NOT NULL,
    last_used_at     TEXT
);
CREATE INDEX credentials_user ON credentials (user_id);

CREATE TABLE sessions (
    token_hash   BLOB    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TEXT    NOT NULL,
    expires_at   TEXT    NOT NULL,
    last_seen_at TEXT    NOT NULL,
    user_agent   TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE webauthn_ceremonies (
    id         TEXT    PRIMARY KEY,
    kind       TEXT    NOT NULL CHECK (kind IN ('register', 'login', 'add', 'recover')),
    user_id    INTEGER REFERENCES users (id) ON DELETE CASCADE,
    data       TEXT    NOT NULL,
    expires_at TEXT    NOT NULL
);
CREATE INDEX webauthn_ceremonies_expires ON webauthn_ceremonies (expires_at);

CREATE TABLE invites (
    token_hash BLOB    PRIMARY KEY,
    created_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    email      TEXT,
    note       TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL,
    expires_at TEXT    NOT NULL,
    used_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    used_at    TEXT
);

CREATE TABLE setup_tokens (
    token_hash BLOB PRIMARY KEY,
    created_at TEXT NOT NULL,
    used_at    TEXT
);

CREATE TABLE email_tokens (
    token_hash BLOB    PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    TEXT    NOT NULL CHECK (purpose IN ('verify', 'recover')),
    email      TEXT    NOT NULL,
    created_at TEXT    NOT NULL,
    expires_at TEXT    NOT NULL,
    used_at    TEXT
);
CREATE INDEX email_tokens_user ON email_tokens (user_id, purpose);

CREATE TABLE code (
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    exercise   TEXT    NOT NULL,
    file       TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    updated_at TEXT    NOT NULL,
    PRIMARY KEY (user_id, exercise, file)
);

CREATE TABLE progress (
    user_id        INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    exercise       TEXT    NOT NULL,
    status         TEXT    NOT NULL CHECK (status IN ('started', 'passed')),
    started_at     TEXT    NOT NULL,
    passed_at      TEXT,
    active_seconds INTEGER NOT NULL DEFAULT 0,
    runs           INTEGER NOT NULL DEFAULT 0,
    last_run_at    TEXT,
    last_result    TEXT,
    notes          TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, exercise)
);

CREATE TABLE reveals (
    user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    exercise    TEXT    NOT NULL,
    level       INTEGER NOT NULL CHECK (level BETWEEN 1 AND 6),
    revealed_at TEXT    NOT NULL,
    PRIMARY KEY (user_id, exercise, level)
);

CREATE TABLE certificates (
    id           TEXT    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL CHECK (kind IN ('tier1', 'tier2', 'tier3', 'tier4', 'tier5', 'course')),
    name_on_cert TEXT    NOT NULL,
    issued_at    TEXT    NOT NULL,
    show_stats   INTEGER NOT NULL DEFAULT 0,
    stats        TEXT,
    payload      TEXT    NOT NULL,
    signature    BLOB    NOT NULL,
    revoked_at   TEXT,
    replaced_by  TEXT    REFERENCES certificates (id)
);
CREATE INDEX certificates_user ON certificates (user_id);
-- At most 1 valid certificate per user and kind; a re-issue revokes the old
-- one in the same transaction.
CREATE UNIQUE INDEX certificates_valid ON certificates (user_id, kind) WHERE revoked_at IS NULL;

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE audit (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    event      TEXT    NOT NULL,
    detail     TEXT    NOT NULL DEFAULT '{}',
    ip         TEXT    NOT NULL DEFAULT '',
    created_at TEXT    NOT NULL
);
CREATE INDEX audit_created ON audit (created_at);
CREATE INDEX audit_user ON audit (user_id, created_at);

-- Readiness writes and reads this row to prove the database is writable.
CREATE TABLE probe (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
