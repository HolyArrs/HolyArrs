-- Stage 2.0 accounts: roles owner, admin and member, per-user permissions,
-- two-factor sign-in, sign-in through Plex, Jellyfin or Emby, and invites.
-- The users table is rebuilt to widen its role check; sessions and API keys
-- are rebuilt with it so no row is lost (dropping a parent table would
-- cascade). The owner keeps its id, name and password.

CREATE TABLE users_new (
    id              INTEGER PRIMARY KEY,
    created_at      TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at      TEXT NOT NULL, updated_by TEXT NOT NULL,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    -- Empty for people who only sign in through Plex, Jellyfin or Emby.
    password_hash   TEXT NOT NULL,
    state           TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'locked', 'disabled')),
    -- JSON list of permissions; the owner has every one whatever this says.
    perms           TEXT NOT NULL DEFAULT '[]',
    -- Two-factor: secret ids in the encrypted secrets table.
    totp_secret_id  INTEGER,
    totp_pending_id INTEGER,
    totp_last_step  INTEGER NOT NULL DEFAULT 0
);
INSERT INTO users_new(id, created_at, created_by, updated_at, updated_by, name, role, password_hash, state, perms)
    SELECT id, created_at, created_by, updated_at, updated_by, name, role, password_hash, state, '[]' FROM users;

CREATE TABLE sessions_new (
    hash          TEXT PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users_new(id) ON DELETE CASCADE,
    csrf          TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    rotated_at    TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    user_agent    TEXT NOT NULL DEFAULT ''
);
INSERT INTO sessions_new SELECT hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent FROM sessions;

CREATE TABLE api_keys_new (
    id            INTEGER PRIMARY KEY,
    created_at    TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at    TEXT NOT NULL, updated_by TEXT NOT NULL,
    owner_user_id INTEGER NOT NULL REFERENCES users_new(id) ON DELETE CASCADE,
    label         TEXT NOT NULL,
    scope         TEXT NOT NULL DEFAULT '["all"]',
    last_used_at  TEXT,
    hash          TEXT NOT NULL UNIQUE,
    hint          TEXT NOT NULL
);
INSERT INTO api_keys_new SELECT id, created_at, created_by, updated_at, updated_by, owner_user_id, label, scope, last_used_at, hash, hint FROM api_keys;

DROP TABLE sessions;
DROP TABLE api_keys;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
ALTER TABLE sessions_new RENAME TO sessions;
ALTER TABLE api_keys_new RENAME TO api_keys;
CREATE UNIQUE INDEX users_one_owner ON users(role) WHERE role = 'owner';
CREATE INDEX sessions_user ON sessions(user_id);

-- A person's account on Plex, Jellyfin or Emby, linked to one user.
CREATE TABLE user_links (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      TEXT NOT NULL CHECK (provider IN ('plex', 'jellyfin', 'emby')),
    external_id   TEXT NOT NULL,
    external_name TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    PRIMARY KEY (provider, external_id)
);
CREATE INDEX user_links_user ON user_links(user_id);

-- One-time invite links. Only the SHA-256 hash of the link's token is kept.
CREATE TABLE invites (
    hash        TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    created_by  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('admin', 'member')),
    perms       TEXT NOT NULL DEFAULT '[]',
    note        TEXT NOT NULL DEFAULT '',
    expires_at  TEXT NOT NULL,
    used_at     TEXT,
    used_by     INTEGER REFERENCES users(id) ON DELETE SET NULL
);

-- Two-factor backup codes, hashed, each usable once.
CREATE TABLE backup_codes (
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hash     TEXT NOT NULL,
    used_at  TEXT,
    PRIMARY KEY (user_id, hash)
);
