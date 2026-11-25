-- The default sound and subtitle tracks HolyArrs picked on a media server,
-- one row per server and file part, with the languages it picked for, so a
-- file is set once per choice and a track someone changes by hand stays.
CREATE TABLE media_tracks (
    media_server_id INTEGER NOT NULL REFERENCES media_servers(id) ON DELETE CASCADE,
    part_key        TEXT NOT NULL,
    choice          TEXT NOT NULL,
    set_at          TEXT NOT NULL,
    PRIMARY KEY (media_server_id, part_key)
);
