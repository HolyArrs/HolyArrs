-- holyarrs:foreign-key-rebuild
-- Widen the adapter check while preserving Downloader identities and every
-- dependent Download. The runner pins one connection, disables FK enforcement
-- outside the transaction, checks every FK before commit, then restores it.

DROP TRIGGER import_created_downloader_resumed;
DROP TRIGGER import_created_downloader_gone;

CREATE TABLE downloaders_new (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    adapter      TEXT NOT NULL CHECK (adapter IN ('qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge')),
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
    removed INTEGER NOT NULL DEFAULT 0
);

INSERT INTO downloaders_new(id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url,
    secret_ref, category, priority, path_map, after_import, seed_goal, state, username, allow_local_network,
    backoff_until, fail_count, last_error, checked_at, removed)
SELECT id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url,
    secret_ref, category, priority, path_map, after_import, seed_goal, state, username, allow_local_network,
    backoff_until, fail_count, last_error, checked_at, removed FROM downloaders;

DROP TABLE downloaders;
ALTER TABLE downloaders_new RENAME TO downloaders;

CREATE TRIGGER import_created_downloader_resumed AFTER UPDATE OF state ON downloaders
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = NEW.id;
END;

-- An append can be accepted even when its response is lost. Keep the identity
-- before sending; an empty later lookup never authorizes another append.
CREATE TABLE downloader_attempts (
    id INTEGER PRIMARY KEY,
    downloader_id INTEGER NOT NULL REFERENCES downloaders(id),
    -- Snapshot ids retain the audit when library cleanup removes the parents.
    release_id INTEGER NOT NULL,
    slot_id INTEGER NOT NULL,
    release_created_at TEXT NOT NULL DEFAULT '',
    slot_created_at TEXT NOT NULL DEFAULT '',
    version_id INTEGER NOT NULL DEFAULT 0,
    version_created_at TEXT NOT NULL DEFAULT '',
    slot_snapshots TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(slot_snapshots)),
    marker TEXT NOT NULL UNIQUE,
    expected_id TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'resolved', 'abandoned')),
    abandoned_by TEXT CHECK (abandoned_by IS NULL OR json_valid(abandoned_by)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX downloader_attempts_unfinished
    ON downloader_attempts(release_id, release_created_at, version_id, version_created_at) WHERE state IN ('pending', 'abandoned');
CREATE INDEX downloader_attempts_by_downloader ON downloader_attempts(downloader_id, state, id);

CREATE TRIGGER import_created_downloader_gone AFTER DELETE ON downloaders
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = OLD.id;
END;
