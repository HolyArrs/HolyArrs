-- New-episode notices: when an episode file arrives for a show, the people
-- who asked for the show hear about it, batched per show per hour. Each
-- person may turn these off (new_episodes = 0).
ALTER TABLE user_ask_settings ADD COLUMN new_episodes INTEGER NOT NULL DEFAULT 1;
CREATE TABLE new_episode_notices (
    id          INTEGER PRIMARY KEY,
    title_id    INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    slot_id     INTEGER NOT NULL UNIQUE,
    arrived_at  TEXT NOT NULL,
    sent_at     TEXT
);
CREATE INDEX new_episode_notices_open ON new_episode_notices(sent_at, title_id);
