-- Weekly ask limits per kind (movies, shows, music) and per person.
-- The shared limit (ask_per_week, with the ask_shows_count and
-- ask_music_count switches) becomes one default per kind: each kind takes
-- the old number, and a kind that did not count before gets 0 (no limit),
-- so nobody's limits change by the upgrade.

INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
SELECT grp, k.key, value, created_at, created_by, updated_at, updated_by
FROM settings, (SELECT 'ask_movies_per_week' AS key UNION ALL SELECT 'ask_shows_per_week' UNION ALL SELECT 'ask_music_per_week') k
WHERE grp = 'this_app' AND settings.key = 'ask_per_week'
ON CONFLICT (grp, key) DO NOTHING;

INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
SELECT grp, 'ask_shows_per_week', '0', created_at, created_by, updated_at, updated_by
FROM settings WHERE grp = 'this_app' AND key = 'ask_shows_count' AND value = 'false'
ON CONFLICT (grp, key) DO UPDATE SET value = '0';

INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
SELECT grp, 'ask_music_per_week', '0', created_at, created_by, updated_at, updated_by
FROM settings WHERE grp = 'this_app' AND key = 'ask_music_count' AND value = 'false'
ON CONFLICT (grp, key) DO UPDATE SET value = '0';

-- Per person: one own limit per kind (NULL takes the kind's default). The
-- older per_week stays as the value for every kind without its own; a kind
-- that did not count before gets no limit for people with their own
-- per_week, as before.
ALTER TABLE user_ask_settings ADD COLUMN movies_per_week INTEGER CHECK (movies_per_week IS NULL OR movies_per_week BETWEEN 0 AND 1000);
ALTER TABLE user_ask_settings ADD COLUMN shows_per_week INTEGER CHECK (shows_per_week IS NULL OR shows_per_week BETWEEN 0 AND 1000);
ALTER TABLE user_ask_settings ADD COLUMN music_per_week INTEGER CHECK (music_per_week IS NULL OR music_per_week BETWEEN 0 AND 1000);

UPDATE user_ask_settings SET shows_per_week = 0
WHERE per_week IS NOT NULL AND EXISTS (SELECT 1 FROM settings WHERE grp = 'this_app' AND key = 'ask_shows_count' AND value = 'false');
UPDATE user_ask_settings SET music_per_week = 0
WHERE per_week IS NOT NULL AND EXISTS (SELECT 1 FROM settings WHERE grp = 'this_app' AND key = 'ask_music_count' AND value = 'false');

-- ask_per_week, ask_shows_count and ask_music_count stay as they were
-- (migrations only add); nothing reads them any more.
