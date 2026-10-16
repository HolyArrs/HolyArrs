-- Stage 2.2 (TV shows): seasons and episodes on the shared Title, Part and
-- Item records (design/domain/shows.md). Add-only: old rows keep working.

-- What was picked at add time for which seasons to keep up to date.
ALTER TABLE titles ADD COLUMN season_choice TEXT
    CHECK (season_choice IS NULL OR season_choice IN ('all', 'future', 'latest', 'none'));
-- Where and when a show airs (from the details source).
ALTER TABLE titles ADD COLUMN network TEXT;
ALTER TABLE titles ADD COLUMN air_time TEXT;
ALTER TABLE titles ADD COLUMN time_zone TEXT;

-- Set when the details source no longer lists the season.
ALTER TABLE parts ADD COLUMN dropped_at TEXT;

-- The local calendar day an episode airs (the key of the daily order);
-- air_date stays the exact air time in UTC.
ALTER TABLE items ADD COLUMN air_day TEXT;
-- Per-source episode ids.
ALTER TABLE items ADD COLUMN source_ids TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(source_ids));
-- Set when the details source no longer lists the episode but a file or
-- download still holds it.
ALTER TABLE items ADD COLUMN dropped_at TEXT;

-- One row per season, and one per (season, episode) of a show.
CREATE UNIQUE INDEX parts_season ON parts(title_id, number) WHERE kind = 'season';
CREATE UNIQUE INDEX items_episode ON items(title_id, json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')) WHERE kind = 'episode';
CREATE INDEX items_part ON items(part_id);
CREATE INDEX items_air ON items(air_date) WHERE kind = 'episode';
CREATE INDEX titles_kind ON titles(kind, state);

-- Show defaults (recommended choices, logged in decisions/log-autonomous.md).
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('what_to_pick', 'show_season_choice', 'all', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'show_new_seasons', 'all', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'show_episode_order', 'aired', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'show_details_source', 'tmdb', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'keep_specials', 'false', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'episode_wait_minutes', '0', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;

-- Show details refresh every 12 hours (Sonarr's show refresh interval).
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'refresh_shows', 43200, 1)
ON CONFLICT (kind) DO NOTHING;
