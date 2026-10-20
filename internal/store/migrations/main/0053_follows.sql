-- Follow: anyone may follow a show or an artist to hear about new episodes,
-- new seasons and new albums, without asking for anything. New albums of an
-- artist are noted per album and told to followers batched per hour.
CREATE TABLE follows (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title_id    INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, title_id)
);
CREATE INDEX follows_title ON follows(title_id);
CREATE TABLE new_album_notices (
    id          INTEGER PRIMARY KEY,
    title_id    INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    part_id     INTEGER NOT NULL UNIQUE,
    arrived_at  TEXT NOT NULL,
    sent_at     TEXT
);
CREATE INDEX new_album_notices_open ON new_album_notices(sent_at, title_id);
