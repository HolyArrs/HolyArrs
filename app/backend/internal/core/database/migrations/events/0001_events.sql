CREATE TABLE events (
    id          INTEGER PRIMARY KEY,
    at          TEXT NOT NULL,
    actor       TEXT NOT NULL,
    kind        TEXT NOT NULL,
    title_id    INTEGER,
    slot_id     INTEGER,
    download_id INTEGER,
    job_id      INTEGER,
    summary     TEXT NOT NULL,
    data        TEXT NOT NULL DEFAULT '{}',
    group_key   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    created_by  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    updated_by  TEXT NOT NULL
);
CREATE INDEX events_at ON events(at);
CREATE INDEX events_title ON events(title_id, at);
