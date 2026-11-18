-- The Plex labels HolyArrs put on asked-for titles, one row per server,
-- title and label, so a label is put on once and a label someone removed
-- in Plex is not put back. Add-only.
CREATE TABLE media_labels (
    media_server_id INTEGER NOT NULL REFERENCES media_servers(id) ON DELETE CASCADE,
    title_id        INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    label           TEXT NOT NULL,
    added_at        TEXT NOT NULL,
    PRIMARY KEY (media_server_id, title_id, label)
);
