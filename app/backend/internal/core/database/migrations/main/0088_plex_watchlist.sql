-- Roadmap #102: Plex watchlist sync. A person turns on reading their Plex
-- watchlist (movies, shows or both); every few minutes HolyArrs reads the
-- newest entries and asks for the new ones as that person, through the
-- normal ask path. Each entry is acted on once (plex_watchlist_items), so a
-- title taken back or turned down is not asked for again. An ask made (or
-- joined) from a watchlist is the entry's ask_id in state asked or joined.
-- Only new tables, so the step can run again safely.

-- Each person's switches (off by default) and their last read.
CREATE TABLE IF NOT EXISTS plex_watchlist_people (
    user_id     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    movies      INTEGER NOT NULL DEFAULT 0,
    shows       INTEGER NOT NULL DEFAULT 0,
    checked_at  TEXT,
    error       TEXT NOT NULL DEFAULT ''
);

-- What was seen on each person's Plex watchlist and what was done with it.
-- state: asked, joined, already (this person asked before), in_library,
-- skipped (Never add or above their age-rating limit), limit (weekly limit
-- reached; tried again), failed (tried again a few times), no_match (no
-- TMDB, TVDB or IMDb id).
CREATE TABLE IF NOT EXISTS plex_watchlist_items (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plex_key    TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('movie', 'show')),
    ref         TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL DEFAULT '',
    year        INTEGER,
    state       TEXT NOT NULL,
    ask_id      INTEGER REFERENCES asks(id) ON DELETE SET NULL,
    note        TEXT NOT NULL DEFAULT '',
    tries       INTEGER NOT NULL DEFAULT 0,
    seen_at     TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (user_id, plex_key)
);
CREATE INDEX IF NOT EXISTS plex_watchlist_items_seen ON plex_watchlist_items(user_id, seen_at);

INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'sync_plex_watchlists', 300, 1)
ON CONFLICT (kind) DO NOTHING;
