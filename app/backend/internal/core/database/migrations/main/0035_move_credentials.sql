-- 3.5.13: old-app keys saved from the owner's secrets file for the Move
-- tool. Each row points at one vault secret (address, key and the local
-- network switch, sealed together). A successful read by the Move tool,
-- or `holyarrs import-secrets -forget-move-credentials`, deletes the row
-- and its secret. Owner only; the key is never answered.
CREATE TABLE move_credentials (
    kind       TEXT PRIMARY KEY CHECK (kind IN ('radarr', 'sonarr', 'lidarr', 'prowlarr', 'bazarr', 'seerr')),
    secret_id  INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL
);
