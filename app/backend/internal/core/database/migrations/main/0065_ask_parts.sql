-- What asks are made of, stored once when asked for: a show's episode
-- list per season (show:<ref>:<season>) and an album's songs
-- (album:<mbid>), JSON. The Approvals panel reads them with the ask.
CREATE TABLE ask_parts (
    key        TEXT PRIMARY KEY,
    body       TEXT NOT NULL,
    fetched_at TEXT NOT NULL
);
