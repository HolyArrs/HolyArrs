ALTER TABLE media_servers ADD COLUMN monitor_playback INTEGER NOT NULL DEFAULT 0 CHECK (monitor_playback IN (0, 1));

CREATE TABLE plex_playback_state (
    media_server_id INTEGER PRIMARY KEY REFERENCES media_servers(id) ON DELETE CASCADE,
    generation      INTEGER NOT NULL DEFAULT 0,
    last_seen_at    TEXT,
    stale           INTEGER NOT NULL DEFAULT 0 CHECK (stale IN (0, 1))
);

CREATE TABLE plex_playbacks (
    id              INTEGER PRIMARY KEY,
    media_server_id INTEGER NOT NULL REFERENCES media_servers(id) ON DELETE CASCADE,
    session_key     TEXT NOT NULL,
    account_key     TEXT NOT NULL,
    item_key        TEXT NOT NULL,
    title_id        INTEGER REFERENCES titles(id) ON DELETE SET NULL,
    title           TEXT NOT NULL,
    person          TEXT NOT NULL,
    device          TEXT NOT NULL,
    method          TEXT NOT NULL,
    state           TEXT NOT NULL,
    position_ms     INTEGER NOT NULL,
    duration_ms     INTEGER NOT NULL,
    watch_ms        INTEGER NOT NULL DEFAULT 0,
    started_at      TEXT NOT NULL,
    seen_at         TEXT NOT NULL,
    ended_at        TEXT,
    incomplete      INTEGER NOT NULL DEFAULT 0 CHECK (incomplete IN (0, 1))
);

CREATE UNIQUE INDEX plex_playbacks_active ON plex_playbacks(media_server_id, session_key, account_key, item_key) WHERE ended_at IS NULL;
CREATE INDEX plex_playbacks_history ON plex_playbacks(media_server_id, started_at DESC, id DESC);
CREATE INDEX plex_playbacks_title ON plex_playbacks(title_id);
