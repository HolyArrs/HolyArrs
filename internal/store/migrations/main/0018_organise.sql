-- Stage 5.1 Organise. A rename writes each step here before it touches the
-- disk, so a rename cut off by a crash is finished on the next run.
CREATE TABLE rename_steps (
    id         INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL,
    job_id     INTEGER,
    file_id    INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    from_path  TEXT NOT NULL,
    to_path    TEXT NOT NULL,
    to_rel     TEXT NOT NULL,
    state      TEXT NOT NULL CHECK (state IN ('pending', 'done', 'failed')),
    error      TEXT
);
CREATE INDEX rename_steps_pending ON rename_steps(state) WHERE state = 'pending';

-- Rescan every movie Library folder daily (Radarr rescans every 24 hours)
-- and empty old days out of the holding folders daily.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'rescan_library', 86400, 1),
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'clean_holding', 86400, 1)
ON CONFLICT (kind) DO NOTHING;
