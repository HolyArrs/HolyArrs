-- The old-app API (roadmap #85, decision 0008): Seerr and Overseerr talk
-- to HolyArrs as if it were Sonarr or Radarr. Each side has at most one
-- key of its own, kept apart from HolyArrs API keys; only the SHA-256
-- hash is stored. A key acts as the person who made it.
CREATE TABLE compat_keys (
    side          TEXT PRIMARY KEY CHECK (side IN ('sonarr', 'radarr')),
    created_at    TEXT NOT NULL, created_by TEXT NOT NULL,
    owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_used_at  TEXT,
    hash          TEXT NOT NULL UNIQUE,
    hint          TEXT NOT NULL
);

-- Tags those apps make (Seerr's per-requester tags). They are kept so the
-- apps can find them again; titles are not linked to them yet. AUTOINCREMENT
-- so a removed tag's id is never handed out again.
CREATE TABLE compat_tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    label      TEXT NOT NULL UNIQUE COLLATE NOCASE,
    created_at TEXT NOT NULL
);
