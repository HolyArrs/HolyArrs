-- Follow a show that is not in the Library yet: the follow is kept by its
-- TMDB id until the show is added, then it becomes an ordinary follow of
-- the Library title (and the person hears that the show landed). Add-only.
CREATE TABLE tmdb_follows (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind = 'show'),
    tmdb_id     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, kind, tmdb_id)
);
CREATE INDEX tmdb_follows_title ON tmdb_follows(kind, tmdb_id);
