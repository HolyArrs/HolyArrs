-- M1 step 1 (Request): release dates and the Release gate on Titles, the
-- Never-add list, a reason next to Library folder health, the Release gate
-- defaults, and the first scheduled jobs. Add-only: old rows keep working.

-- Release dates from the metadata source (UTC dates, NULL when unknown).
ALTER TABLE titles ADD COLUMN cinema_date TEXT;
ALTER TABLE titles ADD COLUMN digital_date TEXT;
ALTER TABLE titles ADD COLUMN physical_date TEXT;
-- Which date drove the Release gate, and when searching may start.
ALTER TABLE titles ADD COLUMN gate_driver TEXT NOT NULL DEFAULT 'none'
    CHECK (gate_driver IN ('announced', 'cinema', 'digital', 'physical', 'cinema_plus_90', 'none'));
ALTER TABLE titles ADD COLUMN gate_opens_at TEXT;
-- Set when the metadata source no longer knows the Title; it is flagged, never deleted.
ALTER TABLE titles ADD COLUMN dropped_at TEXT;

ALTER TABLE library_folders ADD COLUMN health_reason TEXT NOT NULL DEFAULT '';

-- Never-add list (model.md 2.24): Titles that must not be added again.
CREATE TABLE never_add (
    id         INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('movie', 'show', 'artist')),
    source_ids TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(source_ids)),
    name       TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX never_add_tmdb ON never_add(json_extract(source_ids, '$.tmdb'));

-- Release gate defaults for new movies (open choice 1: "released" + 0 days,
-- bulk searches respect the gate). Changing them changes only the default.
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('what_to_pick', 'release_gate', 'released', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'gate_delay_days', '0', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'bulk_search_respects_gate', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;

-- Scheduled jobs: details refresh every 24 h, Library folder health every 6 h.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'refresh_titles', 86400, 1),
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'check_library_folders', 21600, 1)
ON CONFLICT (kind) DO NOTHING;
