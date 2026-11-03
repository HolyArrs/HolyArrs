-- Some databases have 0056 marked done without its table (seen on a
-- database carried across the branch join), so following a movie by its
-- TMDB id failed. Create it again where it is missing; a no-op elsewhere.
CREATE TABLE IF NOT EXISTS tmdb_movie_follows (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tmdb_id     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, tmdb_id)
);
CREATE INDEX IF NOT EXISTS tmdb_movie_follows_title ON tmdb_movie_follows(tmdb_id);
