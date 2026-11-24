-- Clean up old search results once a day (#532). Releases nobody needs any
-- more are removed in small batches; the indexes below let each delete find
-- the rows that point at a Release without reading whole tables.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'clean_releases', 86400, 1)
ON CONFLICT (kind) DO NOTHING;
CREATE INDEX IF NOT EXISTS downloads_release ON downloads(release_id) WHERE release_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS downloader_attempts_release ON downloader_attempts(release_id);
CREATE INDEX IF NOT EXISTS slots_held_release ON slots(held_release_id) WHERE held_release_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS slots_chosen_release ON slots(chosen_release_id) WHERE chosen_release_id IS NOT NULL;
