-- Stage 4.0 (Downloaders, shared): sign-in name, the local-network switch,
-- back-off and the last check per Downloader; a soft remove for Downloaders
-- with Download history; the last-progress time per Download so the stall
-- timers survive a restart; the Downloader settings and the status check.
-- Add-only.
ALTER TABLE downloaders ADD COLUMN username TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN allow_local_network INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloaders ADD COLUMN backoff_until TEXT;
ALTER TABLE downloaders ADD COLUMN fail_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloaders ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN checked_at TEXT;
ALTER TABLE downloaders ADD COLUMN removed INTEGER NOT NULL DEFAULT 0;
-- Downloaders added before this stage came from start flags on the local
-- network (the lab qBittorrent).
UPDATE downloaders SET allow_local_network = 1;

ALTER TABLE downloads ADD COLUMN progress_at TEXT;
UPDATE downloads SET progress_at = updated_at;

-- Stalled after 6 hours with no progress, acted on after 24 (choice 7);
-- failed Downloads are removed from the Downloader with their files;
-- programs inside a Download fail it; a Download removed outside HolyArrs
-- is replaced; sends to one host at least 2 seconds apart.
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
    ('links', 'stalled_after_hours', '6', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'stalled_act_hours', '24', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'remove_failed', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'fail_dangerous', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'replace_vanished', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'),
    ('links', 'grab_spacing_seconds', '2', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}')
ON CONFLICT (grp, key) DO NOTHING;
