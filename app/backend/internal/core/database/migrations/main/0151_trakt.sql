-- Trakt: what the person signed in with Trakt has watched there, and each
-- Library title's Trakt rating.
--
-- trakt_watch_state holds Trakt's watched movies and shows for the Library
-- titles, read with the watch history only while the owner keeps it on.
-- user_id is the person here who signed in. Movies keep season and episode
-- at 0. Create-only.
CREATE TABLE trakt_watch_state (
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_name    TEXT NOT NULL DEFAULT '',
    title_id        INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('movie', 'episode')),
    season          INTEGER NOT NULL DEFAULT 0,
    episode         INTEGER NOT NULL DEFAULT 0,
    plays           INTEGER NOT NULL DEFAULT 1,
    last_watched_at TEXT,
    read_at         TEXT NOT NULL,
    PRIMARY KEY (user_id, title_id, season, episode)
);
CREATE INDEX trakt_watch_state_title ON trakt_watch_state(title_id);

-- The Trakt rating (out of 10) and its vote count, read with the Trakt
-- client id by a slow pass of its own. NULL means not known; trakt_votes
-- is 0 then. trakt_checked_at is when Trakt was last asked, so a title is
-- asked again at most once a week.
ALTER TABLE titles ADD COLUMN trakt_rating REAL;
ALTER TABLE titles ADD COLUMN trakt_votes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE titles ADD COLUMN trakt_checked_at TEXT;
