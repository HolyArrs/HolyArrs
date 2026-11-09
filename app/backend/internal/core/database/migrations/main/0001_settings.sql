CREATE TABLE settings (
    id          INTEGER PRIMARY KEY,
    grp         TEXT NOT NULL CHECK (grp IN ('links', 'where_files_go', 'what_to_pick', 'this_app')),
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    updated_by  TEXT NOT NULL,
    UNIQUE (grp, key)
);
