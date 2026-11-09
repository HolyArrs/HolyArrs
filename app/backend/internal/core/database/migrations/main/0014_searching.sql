-- Stage 3.1 (Search for movies): the feed check, the Held check and the
-- missing search as scheduled Jobs, their settings, and an index for the
-- Held and Turned down lists. Add-only.

-- Feed check every 15 minutes (decision log 3.1.1; the old apps use 15 or
-- 30), Held Releases looked at every minute, and the missing search once a
-- day, first at 03:00 UTC tomorrow.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'feed_check', 900, 1),
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'grab_held', 60, 1)
ON CONFLICT (kind) DO NOTHING;
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled, next_run) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'search_missing', 86400, 1,
     strftime('%Y-%m-%dT03:00:00.000Z', 'now', '+1 day'))
ON CONFLICT (kind) DO NOTHING;

-- Missing search: up to 20 Slots, missing copies only; try the next
-- Downloader when one cannot take a Release.
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('what_to_pick', 'missing_search_count', '20', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'missing_search_better', 'false', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('what_to_pick', 'try_next_downloader', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;

CREATE INDEX releases_state ON releases(state, updated_at);
CREATE INDEX releases_source_name ON releases(source_id, name);
