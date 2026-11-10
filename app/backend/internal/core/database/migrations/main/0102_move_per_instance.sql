-- The import tool reads several copies of one old app (two Sonarrs, say).
-- Each read records the address it came from, and each address keeps its
-- own key. Keys saved before this have address '' and are matched by the
-- address sealed inside them.
ALTER TABLE migrate_snapshots ADD COLUMN address TEXT NOT NULL DEFAULT '';
CREATE TABLE move_credentials_new (
    kind       TEXT NOT NULL CHECK (kind IN ('radarr', 'sonarr', 'lidarr', 'prowlarr', 'bazarr', 'seerr', 'overseerr', 'qbittorrent', 'sabnzbd')),
    address    TEXT NOT NULL DEFAULT '',
    secret_id  INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL,
    PRIMARY KEY (kind, address)
);
INSERT INTO move_credentials_new (kind, address, secret_id, created_at, created_by)
SELECT kind, '', secret_id, created_at, created_by FROM move_credentials;
DROP TABLE move_credentials;
ALTER TABLE move_credentials_new RENAME TO move_credentials;
