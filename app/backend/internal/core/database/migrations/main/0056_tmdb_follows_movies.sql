-- Follow a movie not in the Library yet (often not out yet) by its TMDB id:
-- the person hears once when it lands, then the row goes. Add-only.
CREATE TABLE tmdb_movie_follows (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tmdb_id     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, tmdb_id)
);
CREATE INDEX tmdb_movie_follows_title ON tmdb_movie_follows(tmdb_id);
