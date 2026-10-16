-- Stage 6.2: moves from the old apps. A move is the plan the owner
-- approved (its snapshots and choices), its state and its report.
-- move_made marks every row a move created, so "undo this move" removes
-- only those (never files), and a second move adds nothing twice.
CREATE TABLE moves (
    id          INTEGER PRIMARY KEY,
    created_at  TEXT NOT NULL,
    owner_id    INTEGER NOT NULL,
    snapshots   TEXT NOT NULL DEFAULT '[]',
    choices     TEXT NOT NULL DEFAULT '{}',
    state       TEXT NOT NULL CHECK (state IN ('queued', 'running', 'done', 'failed', 'undone')),
    job_id      INTEGER,
    report      TEXT NOT NULL DEFAULT '{}',
    finished_at TEXT,
    undone_at   TEXT
);

CREATE TABLE move_made (
    move_id    INTEGER NOT NULL REFERENCES moves(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    what       TEXT NOT NULL,
    row_id     INTEGER NOT NULL,
    before     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (move_id, key)
);
CREATE INDEX move_made_key ON move_made(key);
