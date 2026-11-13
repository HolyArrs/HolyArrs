-- holyarrs:foreign-key-rebuild
-- A Source may now read a built-in site directly (format 'site'; the site's
-- id sits in extra). Widen the format check and keep every Source's id, so
-- Releases, calls and health rows keep pointing at it.

DROP TRIGGER import_created_source_resumed;
DROP TRIGGER import_created_source_gone;

CREATE TABLE sources_new (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    protocol     TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet')),
    format       TEXT NOT NULL CHECK (format IN ('newznab', 'torznab', 'rss', 'bridge', 'site')),
    url          TEXT NOT NULL,
    secret_ref   INTEGER,
    use_for      TEXT NOT NULL DEFAULT '{}',
    kinds        TEXT NOT NULL DEFAULT '[]',
    categories   TEXT NOT NULL DEFAULT '{}',
    min_seeders  INTEGER,
    limits       TEXT NOT NULL DEFAULT '{}',
    proxy_id     INTEGER,
    priority     INTEGER NOT NULL DEFAULT 25,
    state        TEXT NOT NULL DEFAULT 'enabled' CHECK (state IN ('enabled', 'paused', 'disabled')),
    pause_reason TEXT CHECK (pause_reason IN ('down', 'rate_limited', 'login_refused', 'blocked', 'broken_definition')),
    paused_until TEXT,
    extra        TEXT NOT NULL DEFAULT '{}',
    allow_local_network INTEGER NOT NULL DEFAULT 0,
    proxy_url TEXT NOT NULL DEFAULT '',
    proxy_secret_ref INTEGER,
    proxy_allow_local_network INTEGER NOT NULL DEFAULT 0,
    path_secret_ref INTEGER,
    tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags))
);

INSERT INTO sources_new(id, created_at, created_by, updated_at, updated_by, name, protocol, format, url, secret_ref,
    use_for, kinds, categories, min_seeders, limits, proxy_id, priority, state, pause_reason, paused_until, extra,
    allow_local_network, proxy_url, proxy_secret_ref, proxy_allow_local_network, path_secret_ref, tags)
SELECT id, created_at, created_by, updated_at, updated_by, name, protocol, format, url, secret_ref,
    use_for, kinds, categories, min_seeders, limits, proxy_id, priority, state, pause_reason, paused_until, extra,
    allow_local_network, proxy_url, proxy_secret_ref, proxy_allow_local_network, path_secret_ref, tags FROM sources;

DROP TABLE sources;
ALTER TABLE sources_new RENAME TO sources;

CREATE TRIGGER import_created_source_resumed AFTER UPDATE OF state ON sources
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'sources' AND item_id = NEW.id;
END;

CREATE TRIGGER import_created_source_gone AFTER DELETE ON sources
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'sources' AND item_id = OLD.id;
END;
