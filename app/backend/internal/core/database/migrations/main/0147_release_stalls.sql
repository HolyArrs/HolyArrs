-- A Release whose Downloads stall in a row (a Downloader behind a dead VPN)
-- waits longer before each new send. count is the stalls in a row, last_at
-- when the newest one failed. A Download of it that finishes clears the row.
CREATE TABLE release_stalls (
    name    TEXT PRIMARY KEY,
    count   INTEGER NOT NULL,
    last_at TEXT NOT NULL
);
