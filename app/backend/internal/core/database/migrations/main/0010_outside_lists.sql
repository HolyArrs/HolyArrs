-- Stage 2.1.9: Outside lists (model.md 2.25). A TMDB or Trakt list adds its
-- movies on a schedule, with why_added = list; the Never-add list wins.
CREATE TABLE outside_lists (
    id                INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name              TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind              TEXT NOT NULL CHECK (kind IN ('tmdb_list', 'trakt_list')),
    ref               TEXT NOT NULL,
    library_folder_id INTEGER NOT NULL REFERENCES library_folders(id),
    pick_rules_id     INTEGER REFERENCES pick_rules(id),
    keep_up_to_date   INTEGER NOT NULL DEFAULT 1,
    search_on_add     INTEGER NOT NULL DEFAULT 0,
    release_gate      TEXT NOT NULL DEFAULT '',
    enabled           INTEGER NOT NULL DEFAULT 1,
    sync_every_hours  INTEGER NOT NULL DEFAULT 12 CHECK (sync_every_hours BETWEEN 6 AND 168),
    last_synced_at    TEXT,
    last_result       TEXT NOT NULL DEFAULT '',
    last_error        TEXT NOT NULL DEFAULT ''
);

-- The list runner looks every 15 minutes; each list waits its own hours.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'sync_outside_lists', 900, 1)
ON CONFLICT (kind) DO NOTHING;
