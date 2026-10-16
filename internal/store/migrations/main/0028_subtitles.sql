-- Stage 4.16 (subtitles): wishes, Subtitle sites, subtitles per File and
-- turned-down subtitles (design/domain/subtitles.md). Add-only.

-- Which subtitles a title, a Version or an episode wants. No row: inherit.
CREATE TABLE subtitle_wishes (
    id         INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    title_id   INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    version_id INTEGER REFERENCES versions(id) ON DELETE CASCADE,
    item_id    INTEGER REFERENCES items(id) ON DELETE CASCADE,
    languages  TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(languages)),
    CHECK (version_id IS NULL OR item_id IS NULL)
);
CREATE UNIQUE INDEX subtitle_wishes_place ON subtitle_wishes(title_id, COALESCE(version_id, 0), COALESCE(item_id, 0));

CREATE TABLE subtitle_sites (
    id                  INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name                TEXT NOT NULL UNIQUE,
    kind                TEXT NOT NULL,
    url                 TEXT NOT NULL,
    secret_ref          INTEGER,
    state               TEXT NOT NULL DEFAULT 'enabled' CHECK (state IN ('enabled', 'paused', 'disabled')),
    pause_reason        TEXT CHECK (pause_reason IN ('down', 'rate_limited', 'download_limit', 'login_refused', 'blocked', 'unreadable')),
    paused_until        TEXT,
    failures            INTEGER NOT NULL DEFAULT 0,
    limits              TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(limits)),
    downloads_today     INTEGER NOT NULL DEFAULT 0,
    downloads_reset_at  TEXT,
    allow_local_network INTEGER NOT NULL DEFAULT 0,
    priority            INTEGER NOT NULL DEFAULT 25
);

-- One row per main File, language, forced and place: what is there (built
-- in, beside), and what the wish still misses.
CREATE TABLE subtitles (
    id               INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    file_id          INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    language         TEXT NOT NULL,
    forced           INTEGER NOT NULL DEFAULT 0,
    hearing_impaired INTEGER NOT NULL DEFAULT 0,
    place            TEXT NOT NULL CHECK (place IN ('built_in', 'beside', 'wish')),
    track_index      INTEGER,
    codec            TEXT,
    relative_path    TEXT,
    site_id          INTEGER REFERENCES subtitle_sites(id) ON DELETE SET NULL,
    site_name        TEXT,
    site_key         TEXT,
    score            INTEGER,
    score_parts      TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(score_parts)),
    origin           TEXT NOT NULL CHECK (origin IN ('found', 'fetched', 'by_hand')),
    state            TEXT NOT NULL CHECK (state IN ('present', 'replaced', 'gone', 'wanted', 'none_good_enough')),
    saved_at         TEXT,
    tries            INTEGER NOT NULL DEFAULT 0,
    first_tried_at   TEXT,
    last_tried_at    TEXT,
    next_try_at      TEXT,
    sync             TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(sync))
);
CREATE INDEX subtitles_file ON subtitles(file_id, state);
CREATE INDEX subtitles_due ON subtitles(state, next_try_at) WHERE state IN ('wanted', 'none_good_enough');
CREATE UNIQUE INDEX subtitles_wish_row ON subtitles(file_id, language, forced) WHERE place = 'wish';
CREATE UNIQUE INDEX subtitles_beside_path ON subtitles(file_id, relative_path) WHERE place = 'beside' AND state = 'present';

CREATE TABLE subtitle_turned_down (
    id         INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    site_name  TEXT NOT NULL,
    site_key   TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    UNIQUE (file_id, site_name, site_key)
);

-- When each main File was last looked at, and for which wish (so a new
-- or changed wish is looked at again, and a met one is not).
CREATE TABLE subtitle_looks (
    file_id   INTEGER PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
    looked_at TEXT NOT NULL,
    wish      TEXT NOT NULL
);

-- Subtitle defaults (Bazarr's, logged in decisions/log-autonomous.md).
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('what_to_pick', 'subtitle_languages', '[]', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'subtitle_pass_mark_movie', '70', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'subtitle_pass_mark_episode', '90', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;

-- Missing subtitles every 6 hours, better subtitles every 12 (Bazarr's).
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'subtitle_missing', 21600, 1),
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'subtitle_better', 43200, 1)
ON CONFLICT (kind) DO NOTHING;
