-- Artist pictures: a 6-hour Job that fills missing artist pictures apart
-- from the MusicBrainz refresh.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'artist_pictures', 21600, 1)
ON CONFLICT (kind) DO NOTHING;
