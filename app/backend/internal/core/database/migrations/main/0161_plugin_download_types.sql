-- holyarrs:foreign-key-rebuild
-- A plugin may bring its own download type (such as 'soulseek') for its
-- Sources, their Releases and its Downloaders (adapter 'plugin'). Widen the
-- protocol checks to torrent, usenet or such a name, keeping every id, row,
-- index and trigger.

DROP TRIGGER import_created_source_resumed;
DROP TRIGGER import_created_source_gone;
DROP TRIGGER import_created_downloader_resumed;
DROP TRIGGER import_created_downloader_gone;

CREATE TABLE sources_new (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    protocol     TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet') OR (length(protocol) BETWEEN 2 AND 31 AND protocol GLOB '[a-z]*' AND protocol NOT GLOB '*[^a-z0-9-]*')),
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
    tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags)),
    downloader_id INTEGER,
    multi_languages TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(multi_languages)),
    no_language_languages TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(no_language_languages))
);

INSERT INTO sources_new(id, created_at, created_by, updated_at, updated_by, name, protocol, format, url, secret_ref,
    use_for, kinds, categories, min_seeders, limits, proxy_id, priority, state, pause_reason, paused_until, extra,
    allow_local_network, proxy_url, proxy_secret_ref, proxy_allow_local_network, path_secret_ref, tags,
    downloader_id, multi_languages, no_language_languages)
SELECT id, created_at, created_by, updated_at, updated_by, name, protocol, format, url, secret_ref,
    use_for, kinds, categories, min_seeders, limits, proxy_id, priority, state, pause_reason, paused_until, extra,
    allow_local_network, proxy_url, proxy_secret_ref, proxy_allow_local_network, path_secret_ref, tags,
    downloader_id, multi_languages, no_language_languages FROM sources;

DROP TABLE sources;
ALTER TABLE sources_new RENAME TO sources;

CREATE TABLE releases_new (
    id               INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    source_id        INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    link_ref         TEXT NOT NULL,
    protocol         TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet') OR (length(protocol) BETWEEN 2 AND 31 AND protocol GLOB '[a-z]*' AND protocol NOT GLOB '*[^a-z0-9-]*')),
    size             INTEGER NOT NULL DEFAULT 0,
    published_at     TEXT,
    seeders          INTEGER,
    peers            INTEGER,
    info_hash        TEXT,
    flags            TEXT NOT NULL DEFAULT '[]',
    parsed           TEXT NOT NULL DEFAULT '{}',
    matched_slot_ids TEXT NOT NULL DEFAULT '[]',
    verdict          TEXT CHECK (verdict IS NULL OR json_valid(verdict)),
    found_by         TEXT NOT NULL CHECK (found_by IN ('feed', 'auto_search', 'hand_search')),
    job_id           INTEGER REFERENCES jobs(id) ON DELETE SET NULL,
    state            TEXT NOT NULL DEFAULT 'seen' CHECK (state IN ('seen', 'judged', 'chosen', 'held', 'passed_over', 'refused', 'turned_down')),
    hold_until       TEXT
);

INSERT INTO releases_new(id, created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol,
    size, published_at, seeders, peers, info_hash, flags, parsed, matched_slot_ids, verdict, found_by, job_id,
    state, hold_until)
SELECT id, created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol,
    size, published_at, seeders, peers, info_hash, flags, parsed, matched_slot_ids, verdict, found_by, job_id,
    state, hold_until FROM releases;

DROP TABLE releases;
ALTER TABLE releases_new RENAME TO releases;

CREATE INDEX releases_source ON releases(source_id);
CREATE INDEX releases_state ON releases(state, updated_at);
CREATE INDEX releases_source_name ON releases(source_id, name);
CREATE INDEX releases_job ON releases(job_id, found_by, matched_slot_ids);

CREATE TABLE downloaders_new (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    adapter      TEXT NOT NULL CHECK (adapter IN ('qbittorrent', 'sabnzbd', 'transmission', 'nzbget', 'deluge', 'rtorrent', 'aria2', 'plugin-torrent', 'plugin-usenet', 'plugin')),
    protocol     TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet') OR (length(protocol) BETWEEN 2 AND 31 AND protocol GLOB '[a-z]*' AND protocol NOT GLOB '*[^a-z0-9-]*')),
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
    category_music TEXT NOT NULL DEFAULT '',
    category_after_import TEXT NOT NULL DEFAULT '',
    own_only INTEGER NOT NULL DEFAULT 0,
    categories_imported TEXT NOT NULL DEFAULT '[]'
);

INSERT INTO downloaders_new(id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url,
    secret_ref, category, priority, path_map, after_import, seed_goal, state, username, allow_local_network,
    backoff_until, fail_count, last_error, checked_at, removed, writable, clear_finished, tags,
    category_movies, category_shows, category_music, category_after_import, own_only, categories_imported)
SELECT id, created_at, created_by, updated_at, updated_by, name, adapter, protocol, url,
    secret_ref, category, priority, path_map, after_import, seed_goal, state, username, allow_local_network,
    backoff_until, fail_count, last_error, checked_at, removed, writable, clear_finished, tags,
    category_movies, category_shows, category_music, category_after_import, own_only, categories_imported FROM downloaders;

DROP TABLE downloaders;
ALTER TABLE downloaders_new RENAME TO downloaders;

CREATE TRIGGER import_created_source_resumed AFTER UPDATE OF state ON sources
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'sources' AND item_id = NEW.id;
END;

CREATE TRIGGER import_created_source_gone AFTER DELETE ON sources
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'sources' AND item_id = OLD.id;
END;

CREATE TRIGGER import_created_downloader_resumed AFTER UPDATE OF state ON downloaders
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = NEW.id;
END;

CREATE TRIGGER import_created_downloader_gone AFTER DELETE ON downloaders
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = OLD.id;
END;
