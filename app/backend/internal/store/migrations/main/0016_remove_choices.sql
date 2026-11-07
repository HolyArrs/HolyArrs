-- Stage 4.1.1: the choices the Remove dialog on the Downloads screen starts
-- from, Radarr's defaults: remove from the Downloader with its files, keep
-- the Release (not turned down), and search again when it is turned down.
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('links', 'remove_delete_files', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'remove_turn_down', 'false', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'remove_search_again', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;
