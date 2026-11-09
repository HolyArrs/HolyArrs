-- Clean up History once a day: routine Events past their keep time (and
-- other Events too, when a keep time is set for them).
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'clean_history', 86400, 1)
ON CONFLICT (kind) DO NOTHING;
