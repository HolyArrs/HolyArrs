-- 5.2 review: an 'alerts' settings group for the Alert settings that are not
-- one Alert's own; pack_alerts moves there from 'links'. SQLite cannot change
-- a CHECK, so the settings table is copied into a new one.
CREATE TABLE settings_new (
    id          INTEGER PRIMARY KEY,
    grp         TEXT NOT NULL CHECK (grp IN ('links', 'where_files_go', 'what_to_pick', 'this_app', 'alerts')),
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    updated_by  TEXT NOT NULL,
    UNIQUE (grp, key)
);
INSERT INTO settings_new (id, grp, key, value, created_at, created_by, updated_at, updated_by)
    SELECT id, CASE WHEN grp = 'links' AND key = 'pack_alerts' THEN 'alerts' ELSE grp END, key, value, created_at, created_by, updated_at, updated_by FROM settings;
DROP TABLE settings;
ALTER TABLE settings_new RENAME TO settings;
