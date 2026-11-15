-- Outside lists get a third kind, mdblist_list: an MDBList list, read with
-- the owner's MDBList key or through its public link. SQLite cannot change a
-- CHECK, so the table is copied. No table points at outside_lists.
CREATE TABLE outside_lists_new (
    id                INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name              TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind              TEXT NOT NULL CHECK (kind IN ('tmdb_list', 'trakt_list', 'mdblist_list')),
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
    last_error        TEXT NOT NULL DEFAULT '',
    title_kind        TEXT NOT NULL DEFAULT 'movie' CHECK (title_kind IN ('movie', 'show')),
    seasons           TEXT NOT NULL DEFAULT '',
    new_seasons       TEXT NOT NULL DEFAULT ''
);
INSERT INTO outside_lists_new (id, created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, pick_rules_id,
    keep_up_to_date, search_on_add, release_gate, enabled, sync_every_hours, last_synced_at, last_result, last_error, title_kind, seasons, new_seasons)
SELECT id, created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, pick_rules_id,
    keep_up_to_date, search_on_add, release_gate, enabled, sync_every_hours, last_synced_at, last_result, last_error, title_kind, seasons, new_seasons FROM outside_lists;
DROP TABLE outside_lists;
ALTER TABLE outside_lists_new RENAME TO outside_lists;
