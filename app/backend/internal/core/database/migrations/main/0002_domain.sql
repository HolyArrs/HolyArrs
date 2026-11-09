-- Domain tables for the walking skeleton (design/domain/model.md).
-- Every row has id, created_at, created_by (actor JSON), updated_at, updated_by.
-- Times are UTC text (RFC 3339). JSON columns hold text.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    created_at    TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at    TEXT NOT NULL, updated_by TEXT NOT NULL,
    name          TEXT NOT NULL UNIQUE,
    role          TEXT NOT NULL CHECK (role IN ('owner')),
    password_hash TEXT NOT NULL,
    state         TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'locked', 'disabled'))
);
-- v1 has one owner.
CREATE UNIQUE INDEX users_one_owner ON users(role) WHERE role = 'owner';

CREATE TABLE pick_rules (
    id         INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name       TEXT NOT NULL UNIQUE,
    kinds      TEXT NOT NULL DEFAULT '[]',
    revision   INTEGER NOT NULL DEFAULT 1,
    preset     INTEGER NOT NULL DEFAULT 0,
    body       TEXT NOT NULL CHECK (json_valid(body))
);

CREATE TABLE library_folders (
    id                    INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    path                  TEXT NOT NULL UNIQUE,
    kind                  TEXT NOT NULL CHECK (kind IN ('movie', 'show', 'music')),
    label                 TEXT NOT NULL,
    default_pick_rules_id INTEGER REFERENCES pick_rules(id),
    free_space            INTEGER NOT NULL DEFAULT 0,
    health                TEXT NOT NULL DEFAULT 'unknown' CHECK (health IN ('ok', 'missing', 'read_only', 'no_hardlink', 'unknown')),
    health_checked_at     TEXT,
    same_disk_as          TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE titles (
    id                INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind              TEXT NOT NULL CHECK (kind IN ('movie', 'show', 'artist')),
    name              TEXT NOT NULL,
    sort_name         TEXT NOT NULL,
    original_name     TEXT,
    year              INTEGER,
    overview          TEXT,
    original_language TEXT,
    aliases           TEXT NOT NULL DEFAULT '[]',
    source_ids        TEXT NOT NULL DEFAULT '{}',
    metadata_source   TEXT NOT NULL CHECK (metadata_source IN ('tmdb', 'tvmaze', 'tvdb', 'musicbrainz', 'local')),
    episode_order     TEXT CHECK (episode_order IN ('aired', 'absolute', 'dvd', 'daily', 'provider_alt')),
    keep_up_to_date   INTEGER NOT NULL DEFAULT 1,
    new_parts_policy  TEXT NOT NULL DEFAULT 'all' CHECK (new_parts_policy IN ('all', 'future_only', 'none')),
    release_gate      TEXT NOT NULL DEFAULT 'released' CHECK (release_gate IN ('announced', 'in_cinemas', 'released')),
    gate_delay_days   INTEGER NOT NULL DEFAULT 0,
    music_types       TEXT NOT NULL DEFAULT '[]',
    why_added         TEXT NOT NULL CHECK (why_added IN ('by_hand', 'list', 'series_follow', 'ask', 'import', 'migration')),
    artwork           TEXT NOT NULL DEFAULT '[]',
    refreshed_at      TEXT,
    status            TEXT NOT NULL DEFAULT 'announced' CHECK (status IN ('announced', 'running', 'ended', 'released')),
    state             TEXT NOT NULL DEFAULT 'added' CHECK (state IN ('added', 'looked_up', 'active', 'removed', 'lookup_failed'))
);
CREATE INDEX titles_name ON titles(sort_name);

CREATE TABLE parts (
    id              INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    title_id        INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('season', 'album')),
    number          INTEGER,
    name            TEXT NOT NULL,
    release_type    TEXT,
    release_date    TEXT,
    keep_up_to_date INTEGER NOT NULL DEFAULT 1,
    source_ids      TEXT NOT NULL DEFAULT '{}',
    pressings       TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX parts_title ON parts(title_id);

CREATE TABLE items (
    id               INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    title_id         INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    part_id          INTEGER REFERENCES parts(id) ON DELETE CASCADE,
    kind             TEXT NOT NULL CHECK (kind IN ('movie', 'episode', 'track')),
    numbers          TEXT NOT NULL DEFAULT '{}',
    name             TEXT,
    air_date         TEXT,
    runtime          INTEGER,
    keep_up_to_date  INTEGER NOT NULL DEFAULT 1,
    last_searched_at TEXT,
    recording_id     TEXT
);
CREATE INDEX items_title ON items(title_id);

CREATE TABLE versions (
    id                INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    title_id          INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    label             TEXT NOT NULL,
    pick_rules_id     INTEGER NOT NULL REFERENCES pick_rules(id),
    library_folder_id INTEGER NOT NULL REFERENCES library_folders(id),
    folder_name       TEXT NOT NULL,
    keep_up_to_date   INTEGER NOT NULL DEFAULT 1,
    is_main           INTEGER NOT NULL DEFAULT 0,
    UNIQUE (title_id, label)
);

CREATE TABLE sources (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    protocol     TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet')),
    format       TEXT NOT NULL CHECK (format IN ('newznab', 'torznab', 'rss', 'bridge')),
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
    extra        TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE jobs (
    id            INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind          TEXT NOT NULL,
    subject       TEXT,
    params        TEXT NOT NULL DEFAULT '{}',
    priority      TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('now', 'normal', 'idle')),
    state         TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'done', 'failed', 'cancelled')),
    progress      REAL NOT NULL DEFAULT 0,
    progress_note TEXT,
    attempt       INTEGER NOT NULL DEFAULT 0,
    max_attempts  INTEGER NOT NULL DEFAULT 5,
    run_after     TEXT NOT NULL,
    started_at    TEXT,
    ended_at      TEXT,
    result        TEXT,
    error         TEXT,
    asked_by      TEXT NOT NULL,
    dedupe_key    TEXT
);
-- One live job per dedupe key.
CREATE UNIQUE INDEX jobs_live_dedupe ON jobs(dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('queued', 'running');
CREATE INDEX jobs_next ON jobs(state, priority, run_after);

CREATE TABLE schedules (
    id            INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind          TEXT NOT NULL UNIQUE,
    every_seconds INTEGER NOT NULL CHECK (every_seconds > 0),
    last_run      TEXT,
    next_run      TEXT,
    enabled       INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE releases (
    id               INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    source_id        INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    link_ref         TEXT NOT NULL,
    protocol         TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet')),
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
CREATE INDEX releases_source ON releases(source_id);

CREATE TABLE downloaders (
    id           INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name         TEXT NOT NULL UNIQUE,
    adapter      TEXT NOT NULL CHECK (adapter IN ('qbittorrent', 'sabnzbd')),
    protocol     TEXT NOT NULL CHECK (protocol IN ('torrent', 'usenet')),
    url          TEXT NOT NULL,
    secret_ref   INTEGER,
    category     TEXT NOT NULL DEFAULT 'holyarrs',
    priority     INTEGER NOT NULL DEFAULT 1,
    path_map     TEXT NOT NULL DEFAULT '[]',
    after_import TEXT NOT NULL DEFAULT 'leave' CHECK (after_import IN ('leave', 'remove', 'recategorize')),
    seed_goal    TEXT NOT NULL DEFAULT '{}',
    state        TEXT NOT NULL DEFAULT 'ok' CHECK (state IN ('ok', 'unreachable', 'auth_failed', 'disabled'))
);

CREATE TABLE downloads (
    id            INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    downloader_id INTEGER NOT NULL REFERENCES downloaders(id),
    release_id    INTEGER REFERENCES releases(id) ON DELETE SET NULL,
    slot_ids      TEXT NOT NULL DEFAULT '[]',
    external_id   TEXT NOT NULL,
    progress      REAL NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 1),
    speed         INTEGER NOT NULL DEFAULT 0,
    eta           INTEGER,
    state         TEXT NOT NULL DEFAULT 'sent' CHECK (state IN ('sent', 'queued', 'downloading', 'finished', 'stalled', 'paused', 'failed', 'vanished', 'seeding', 'cleaned')),
    state_reason  TEXT,
    files_seen    TEXT NOT NULL DEFAULT '[]',
    save_path     TEXT NOT NULL DEFAULT '',
    started_at    TEXT,
    finished_at   TEXT,
    started_by    TEXT NOT NULL,
    UNIQUE (downloader_id, external_id)
);

CREATE TABLE imports (
    id          INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    download_id INTEGER REFERENCES downloads(id) ON DELETE SET NULL,
    from_path   TEXT NOT NULL,
    kind        TEXT NOT NULL,
    plan        TEXT NOT NULL DEFAULT '[]',
    mode        TEXT NOT NULL CHECK (mode IN ('auto', 'dry_run', 'by_hand')),
    result      TEXT NOT NULL DEFAULT '[]',
    decided_by  TEXT,
    state       TEXT NOT NULL DEFAULT 'planned' CHECK (state IN ('planned', 'waiting_for_you', 'approved', 'placing', 'done', 'partly_done', 'failed', 'rejected'))
);

CREATE TABLE files (
    id             INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    slot_ids       TEXT NOT NULL DEFAULT '[]',
    version_id     INTEGER NOT NULL REFERENCES versions(id),
    relative_path  TEXT NOT NULL,
    size           INTEGER NOT NULL DEFAULT 0,
    added_at       TEXT NOT NULL,
    quality        TEXT NOT NULL DEFAULT '{}',
    languages      TEXT NOT NULL DEFAULT '[]',
    media_info     TEXT NOT NULL DEFAULT '{}',
    release_name   TEXT,
    release_group  TEXT,
    origin         TEXT NOT NULL CHECK (origin IN ('imported', 'adopted', 'found')),
    placed_by      TEXT NOT NULL CHECK (placed_by IN ('hardlink', 'move', 'copy', 'none')),
    fell_back      INTEGER NOT NULL DEFAULT 0,
    fingerprint    TEXT,
    checked_at     TEXT,
    role           TEXT NOT NULL DEFAULT 'main' CHECK (role IN ('main', 'subtitle', 'artwork', 'info', 'lyrics')),
    parent_file_id INTEGER REFERENCES files(id) ON DELETE CASCADE,
    state          TEXT NOT NULL DEFAULT 'present' CHECK (state IN ('present', 'missing', 'gone', 'replaced')),
    UNIQUE (version_id, relative_path)
);

CREATE TABLE slots (
    id                 INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    item_id            INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    version_id         INTEGER NOT NULL REFERENCES versions(id) ON DELETE CASCADE,
    state              TEXT NOT NULL CHECK (state IN ('not_out_yet', 'wanted', 'searching', 'held', 'chosen', 'downloading', 'stalled', 'failed', 'importing', 'needs_a_decision', 'rejected', 'in_library', 'better_wanted', 'removed')),
    file_id            INTEGER REFERENCES files(id) ON DELETE SET NULL,
    good_enough        INTEGER NOT NULL DEFAULT 0,
    held_release_id    INTEGER REFERENCES releases(id) ON DELETE SET NULL,
    active_download_id INTEGER REFERENCES downloads(id) ON DELETE SET NULL,
    last_reason        TEXT,
    next_try_at        TEXT,
    UNIQUE (item_id, version_id)
);
CREATE INDEX slots_state ON slots(state, next_try_at);
