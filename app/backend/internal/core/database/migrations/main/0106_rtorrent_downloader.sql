-- holyarrs:foreign-key-rebuild
-- Widen the adapter check to add rTorrent while preserving Downloader
-- identities and every dependent Download. The runner pins one connection,
-- disables FK enforcement outside the transaction, checks every FK before
-- commit, then restores it.

DROP TRIGGER import_created_downloader_resumed;
DROP TRIGGER import_created_downloader_gone;

CREATE TABLE downloaders_new (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    adapter      TEXT NOT NULL CHECK (adapter IN ('qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge', 'rtorrent')),
    protocol     TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet')),
    url          TEXT NOT NULL,
    secret_ref   INTEGER,
    category     TEXT NOT NULL DEFAULT 'holyarrs',
    priority     INTEGER NOT NULL DEFAULT 1,
    path_map     TEXT NOT NULL DEFAULT '[]',
    after_import TEXT NOT NULL DEFAULT 'leave' CHECK (after_import IN ('leave', 'remove', 'recategorize')),
    seed_goal    TEXT NOT NULL DEFAULT '{}',
    state        TEXT NOT NULL DEFAULT 'ok' CHECK (state IN ('ok', 'unreachable', 'auth_failed', 'disabled')),
    username TEXT NOT NULL DEFAULT '',
    allow_local_network INTEGER NOT NULL DEFAULT 0,
    backoff_until TEXT,
    fail_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    checked_at TEXT,
    removed INTEGER NOT NULL DEFAULT 0,
    writable INTEGER NOT NULL DEFAULT 1,
    clear_finished INTEGER NOT NULL DEFAULT 1,
    tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags)),
    category_movies TEXT NOT NULL DEFAULT '',
    category_shows TEXT NOT NULL DEFAULT '',
    category_music TEXT NOT NULL DEFAULT ''
);

INSERT INTO downloaders_new(id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url,
    secret_ref, category, priority, path_map, after_import, seed_goal, state, username, allow_local_network,
    backoff_until, fail_count, last_error, checked_at, removed, writable, clear_finished, tags,
    category_movies, category_shows, category_music)
SELECT id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url,
    secret_ref, category, priority, path_map, after_import, seed_goal, state, username, allow_local_network,
    backoff_until, fail_count, last_error, checked_at, removed, writable, clear_finished, tags,
    category_movies, category_shows, category_music FROM downloaders;

DROP TABLE downloaders;
ALTER TABLE downloaders_new RENAME TO downloaders;

CREATE TRIGGER import_created_downloader_resumed AFTER UPDATE OF state ON downloaders
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = NEW.id;
END;

CREATE TRIGGER import_created_downloader_gone AFTER DELETE ON downloaders
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = OLD.id;
END;
