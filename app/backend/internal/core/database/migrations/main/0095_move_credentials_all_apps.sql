-- The Move tool keeps the key (or qBittorrent sign-in) of every old app it
-- reads, for "Read again", until a move finishes: Overseerr, qBittorrent
-- and SABnzbd join the six apps the secrets file could save.
CREATE TABLE move_credentials_new (
    kind       TEXT PRIMARY KEY CHECK (kind IN ('radarr', 'sonarr', 'lidarr', 'prowlarr', 'bazarr', 'seerr', 'overseerr', 'qbittorrent', 'sabnzbd')),
    secret_id  INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL
);
INSERT INTO move_credentials_new (kind, secret_id, created_at, created_by)
SELECT kind, secret_id, created_at, created_by FROM move_credentials;
DROP TABLE move_credentials;
ALTER TABLE move_credentials_new RENAME TO move_credentials;
