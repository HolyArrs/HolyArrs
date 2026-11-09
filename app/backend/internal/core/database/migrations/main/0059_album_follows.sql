-- Follow an album that is not out yet, by its MusicBrainz release group:
-- the person hears once on its release day, then the row goes. Add-only.
CREATE TABLE album_follows (
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mbid         TEXT NOT NULL,
    name         TEXT NOT NULL,
    artist       TEXT NOT NULL,
    release_date TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    PRIMARY KEY (user_id, mbid)
);
CREATE INDEX album_follows_date ON album_follows(release_date);
