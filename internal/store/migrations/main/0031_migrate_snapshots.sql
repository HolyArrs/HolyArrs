-- Stage 6.1: what the move tool read from an old app (Radarr, Sonarr,
-- Lidarr, Prowlarr, Bazarr, Seerr, Overseerr, qBittorrent, SABnzbd).
-- data is the snapshot as JSON with every secret left out; size is its
-- length in bytes (capped by the app).
CREATE TABLE migrate_snapshots (
    id          INTEGER PRIMARY KEY,
    created_at  TEXT NOT NULL,
    owner_id    INTEGER NOT NULL,
    app_kind    TEXT NOT NULL,
    via         TEXT NOT NULL CHECK (via IN ('api', 'backup')),
    app_version TEXT NOT NULL DEFAULT '',
    counts      TEXT NOT NULL DEFAULT '{}',
    size        INTEGER NOT NULL,
    data        TEXT NOT NULL
);
CREATE INDEX migrate_snapshots_created ON migrate_snapshots(created_at);
