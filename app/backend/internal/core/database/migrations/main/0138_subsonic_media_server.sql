-- holyarrs:foreign-key-rebuild
-- Widen the media server kind check to add Subsonic-style music servers
-- (Navidrome and others), keeping every id so watch state and Plex labels
-- stay linked.

CREATE TABLE media_servers_new (
    id              INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind            TEXT NOT NULL CHECK (kind IN ('plex', 'jellyfin', 'emby', 'subsonic')),
    url             TEXT NOT NULL,
    token_secret_id INTEGER,
    enabled         INTEGER NOT NULL DEFAULT 1,
    on_import       INTEGER NOT NULL DEFAULT 1,
    on_rename       INTEGER NOT NULL DEFAULT 1,
    on_delete       INTEGER NOT NULL DEFAULT 1,
    path_from       TEXT NOT NULL DEFAULT '',
    path_to         TEXT NOT NULL DEFAULT '',
    allow_local_network INTEGER NOT NULL DEFAULT 1,
    last_ok_at      TEXT,
    last_error      TEXT NOT NULL DEFAULT '',
    web_url TEXT NOT NULL DEFAULT ''
);

INSERT INTO media_servers_new(id, created_at, created_by, updated_at, updated_by, name, kind, url, token_secret_id,
    enabled, on_import, on_rename, on_delete, path_from, path_to, allow_local_network, last_ok_at, last_error, web_url)
SELECT id, created_at, created_by, updated_at, updated_by, name, kind, url, token_secret_id,
    enabled, on_import, on_rename, on_delete, path_from, path_to, allow_local_network, last_ok_at, last_error, web_url
FROM media_servers;

DROP TABLE media_servers;
ALTER TABLE media_servers_new RENAME TO media_servers;
