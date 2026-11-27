-- The arr API gets a Lidarr side (#849), so Prowlarr can push its music
-- indexers as music Sources. Its key is kept like the other two. SQLite
-- cannot change a CHECK, so the table is copied. No table points at
-- compat_keys.
CREATE TABLE compat_keys_new (
    side          TEXT PRIMARY KEY CHECK (side IN ('sonarr', 'radarr', 'lidarr')),
    created_at    TEXT NOT NULL, created_by TEXT NOT NULL,
    owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_used_at  TEXT,
    hash          TEXT NOT NULL UNIQUE,
    hint          TEXT NOT NULL
);
INSERT INTO compat_keys_new (side, created_at, created_by, owner_user_id, last_used_at, hash, hint)
SELECT side, created_at, created_by, owner_user_id, last_used_at, hash, hint FROM compat_keys;
DROP TABLE compat_keys;
ALTER TABLE compat_keys_new RENAME TO compat_keys;
