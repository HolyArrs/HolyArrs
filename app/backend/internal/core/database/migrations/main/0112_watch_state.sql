-- What each account on a Plex, Jellyfin or Emby server has watched, read on
-- a schedule only while the owner keeps watch history on. account_key is
-- the server's own account id and account_name its name there; a person is
-- linked through user_links (provider = the server's kind). Movies keep
-- season and episode at 0. Create-only.
CREATE TABLE watch_state (
    media_server_id INTEGER NOT NULL REFERENCES media_servers(id) ON DELETE CASCADE,
    account_key     TEXT NOT NULL,
    account_name    TEXT NOT NULL DEFAULT '',
    title_id        INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('movie', 'episode')),
    season          INTEGER NOT NULL DEFAULT 0,
    episode         INTEGER NOT NULL DEFAULT 0,
    plays           INTEGER NOT NULL DEFAULT 1,
    last_watched_at TEXT,
    read_at         TEXT NOT NULL,
    PRIMARY KEY (media_server_id, account_key, title_id, season, episode)
);
CREATE INDEX watch_state_title ON watch_state(title_id);
