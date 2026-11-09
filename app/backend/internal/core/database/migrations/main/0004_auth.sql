-- Sign-in: sessions and API keys for the one owner (design/domain/model.md 2.21).
-- Session and key values are never stored; only their SHA-256 hashes are.

CREATE TABLE sessions (
    hash          TEXT PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf          TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    rotated_at    TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    user_agent    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE api_keys (
    id            INTEGER PRIMARY KEY,
    created_at    TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at    TEXT NOT NULL, updated_by TEXT NOT NULL,
    owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label         TEXT NOT NULL,
    scope         TEXT NOT NULL DEFAULT '["all"]',
    last_used_at  TEXT,
    hash          TEXT NOT NULL UNIQUE,
    hint          TEXT NOT NULL
);
