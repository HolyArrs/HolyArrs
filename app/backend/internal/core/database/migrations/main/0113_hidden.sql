-- Titles and genres kept out of browsing (Explore, New & Popular, genre
-- pages, search, suggestions, More like this, people pages). A row hides one
-- title (tmdb_id) or one genre of a kind (genre, the English TMDB name), for
-- the whole household (user_id NULL) or for one person. Links, the Library
-- and asks are not affected. Create-only.
CREATE TABLE hidden (
    id         INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('movie', 'show')),
    tmdb_id    TEXT NOT NULL DEFAULT '',
    genre      TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,
    CHECK ((tmdb_id = '') != (genre = ''))
);
CREATE UNIQUE INDEX hidden_one ON hidden(kind, tmdb_id, genre, ifnull(user_id, 0));
