-- Stage 4.16 review: a subtitle save is written here before the disk
-- changes and removed with the subtitle's row. A save cut off between the
-- two leaves its row here; the next look finishes it, so HolyArrs's own
-- file is never mistaken for one someone else put there.
CREATE TABLE subtitle_saves (
    id            INTEGER PRIMARY KEY,
    created_at    TEXT NOT NULL,
    file_id       INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    relative_path TEXT NOT NULL,
    row           TEXT NOT NULL,
    replace_ids   TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX subtitle_saves_file ON subtitle_saves(file_id);
