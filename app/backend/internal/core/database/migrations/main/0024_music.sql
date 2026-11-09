-- Stage 2.3 (music): artists, albums, Pressings and tracks on the shared
-- Title, Part and Item records (design/domain/music.md). Add-only: old rows
-- keep working.

-- Artists: what was picked at add time for which albums to keep up to date,
-- which extra album kinds and Pressing statuses count (the main kinds use
-- the existing music_types column), and descriptive details.
ALTER TABLE titles ADD COLUMN album_choice TEXT
    CHECK (album_choice IS NULL OR album_choice IN ('all', 'future', 'missing', 'latest', 'first', 'none'));
ALTER TABLE titles ADD COLUMN music_secondary TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(music_secondary));
ALTER TABLE titles ADD COLUMN music_statuses TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(music_statuses));
ALTER TABLE titles ADD COLUMN artist_type TEXT;
ALTER TABLE titles ADD COLUMN disambiguation TEXT;

-- Albums (MusicBrainz release groups): extra kinds, whether the album
-- passes the artist's album kinds, and a note that tells same-named
-- albums apart.
ALTER TABLE parts ADD COLUMN secondary_types TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(secondary_types));
ALTER TABLE parts ADD COLUMN included INTEGER NOT NULL DEFAULT 1;
ALTER TABLE parts ADD COLUMN disambiguation TEXT;

-- Tracks: length in milliseconds.
ALTER TABLE items ADD COLUMN length_ms INTEGER;

-- One edition of an album (a MusicBrainz release). Exactly one per album
-- is chosen; the album's tracks are the chosen Pressing's tracks.
CREATE TABLE pressings (
    id          INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    part_id     INTEGER NOT NULL REFERENCES parts(id) ON DELETE CASCADE,
    mbid        TEXT NOT NULL,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT '',
    country     TEXT NOT NULL DEFAULT '',
    date        TEXT NOT NULL DEFAULT '',
    disambiguation TEXT NOT NULL DEFAULT '',
    formats     TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(formats)),
    track_count INTEGER NOT NULL DEFAULT 0,
    tracks      TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(tracks)),
    chosen      INTEGER NOT NULL DEFAULT 0,
    chosen_by   TEXT NOT NULL DEFAULT 'auto' CHECK (chosen_by IN ('auto', 'person')),
    dropped_at  TEXT
);
CREATE UNIQUE INDEX pressings_mbid ON pressings(part_id, mbid);
CREATE UNIQUE INDEX pressings_chosen ON pressings(part_id) WHERE chosen = 1;

-- One album row per (artist, MusicBrainz id); tracks are found by album
-- and by recording.
CREATE UNIQUE INDEX parts_album ON parts(title_id, json_extract(source_ids, '$.musicbrainz')) WHERE kind = 'album';
CREATE INDEX items_recording ON items(recording_id) WHERE kind = 'track';

-- Provider answers cached with an expiry (model.md 2.19). Not the person's
-- data: it can be emptied at any time and is rebuilt on demand.
CREATE TABLE lookup_cache (
    key        TEXT PRIMARY KEY,
    body       BLOB NOT NULL,
    fetched_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX lookup_cache_expiry ON lookup_cache(expires_at);

-- Music defaults (recommended choices, logged in decisions/log-autonomous.md)
-- and where MusicBrainz is reached (decision 0006).
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('what_to_pick', 'music_album_choice', 'all', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'music_new_albums', 'all', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'music_album_types', '["album","ep"]', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'music_secondary_types', '["studio"]', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'music_release_statuses', '["official"]', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'music_pressing_rule', 'common', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'musicbrainz_url', '', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'musicbrainz_cache_hours', '24', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;

-- Artist details refresh every 12 hours (as shows).
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'refresh_artists', 43200, 1)
ON CONFLICT (kind) DO NOTHING;
