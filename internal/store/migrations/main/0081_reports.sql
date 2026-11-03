-- Reports (#95): a person tells the owner and admins about a problem with a
-- title in the Library (bad picture or sound, missing subtitles, a file
-- that will not play). A report can narrow to an album, a season, an
-- episode or track, and a Version; file_id keeps the file that was there
-- when it was made. Two states: open and fixed (a fixed report can be
-- opened again). A second person's report on the same thing is its own row.
CREATE TABLE reports (
    id          INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    title_id    INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    album_id    INTEGER REFERENCES parts(id) ON DELETE SET NULL,
    season      INTEGER,
    item_id     INTEGER REFERENCES items(id) ON DELETE SET NULL,
    version_id  INTEGER REFERENCES versions(id) ON DELETE SET NULL,
    file_id     INTEGER REFERENCES files(id) ON DELETE SET NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('picture', 'sound', 'subtitles', 'wont_play', 'other')),
    note        TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'fixed')),
    made_by     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    closed_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
    closed_at   TEXT,
    reply       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX reports_open ON reports(state) WHERE state = 'open';
CREATE INDEX reports_title ON reports(title_id);
CREATE INDEX reports_made_by ON reports(made_by, id);
-- One open report per person for the same problem on the same thing (and Version).
CREATE UNIQUE INDEX reports_one_open ON reports(made_by, title_id, IFNULL(album_id, 0), IFNULL(season, -1), IFNULL(item_id, 0), IFNULL(version_id, 0), kind) WHERE state = 'open';

-- The reporter's in-app message when a report is fixed.
ALTER TABLE messages ADD COLUMN report_id INTEGER REFERENCES reports(id) ON DELETE SET NULL;
