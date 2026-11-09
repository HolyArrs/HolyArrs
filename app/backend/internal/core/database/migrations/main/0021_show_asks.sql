-- Stage 2.2.7 to 2.2.10: asks for shows. An ask is now for a movie or a
-- show. A show ask keeps the id it was asked by (ref, such as tmdb:1399 or
-- tvmaze:82), every id the details source knows (source_ids, to find the
-- same show asked by another id) and the seasons asked for (seasons, a
-- JSON list; empty is the whole show). tmdb_id stays the TMDB number when
-- known.
--
-- asks is rebuilt to widen the kind check. ask_people and messages point
-- at asks, so they are rebuilt with it (children first, so no row is lost
-- to ON DELETE actions); TestShowAsksMigrationKeepsAsks proves it.

CREATE TABLE asks_new (
    id            INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind          TEXT NOT NULL DEFAULT 'movie' CHECK (kind IN ('movie', 'show')),
    tmdb_id       TEXT NOT NULL,
    name          TEXT NOT NULL,
    year          INTEGER,
    poster        TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL CHECK (state IN ('asked', 'approved', 'turned_down', 'available')),
    title_id      INTEGER REFERENCES titles(id) ON DELETE SET NULL,
    asked_by      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    asked_at      TEXT NOT NULL,
    decided_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    decided_at    TEXT,
    note          TEXT NOT NULL DEFAULT '',
    auto_approved INTEGER NOT NULL DEFAULT 0,
    available_at  TEXT,
    ref           TEXT NOT NULL DEFAULT '',
    source_ids    TEXT NOT NULL DEFAULT '{}',
    seasons       TEXT NOT NULL DEFAULT '[]'
);
CREATE TABLE ask_people_new (
    ask_id    INTEGER NOT NULL REFERENCES asks_new(id) ON DELETE CASCADE,
    user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    asked_at  TEXT NOT NULL,
    counted   INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (ask_id, user_id)
);
CREATE TABLE messages_new (
    id          INTEGER PRIMARY KEY,
    created_at  TEXT NOT NULL,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    text        TEXT NOT NULL,
    ask_id      INTEGER REFERENCES asks_new(id) ON DELETE SET NULL,
    title_id    INTEGER REFERENCES titles(id) ON DELETE SET NULL,
    read_at     TEXT
);

INSERT INTO asks_new (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, year, poster, state, title_id,
    asked_by, asked_at, decided_by, decided_at, note, auto_approved, available_at, ref)
SELECT id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, year, poster, state, title_id,
    asked_by, asked_at, decided_by, decided_at, note, auto_approved, available_at, 'tmdb:' || tmdb_id FROM asks;
UPDATE asks_new SET source_ids = json_object('tmdb', tmdb_id);
INSERT INTO ask_people_new (ask_id, user_id, asked_at, counted) SELECT ask_id, user_id, asked_at, counted FROM ask_people;
INSERT INTO messages_new (id, created_at, user_id, kind, text, ask_id, title_id, read_at)
SELECT id, created_at, user_id, kind, text, ask_id, title_id, read_at FROM messages;

DROP TABLE messages;
DROP TABLE ask_people;
DROP TABLE asks;
ALTER TABLE asks_new RENAME TO asks;
ALTER TABLE ask_people_new RENAME TO ask_people;
ALTER TABLE messages_new RENAME TO messages;

-- One open ask per movie (a show may have one open ask per set of seasons
-- not yet kept; the asks package finds the same show by any of its ids).
CREATE UNIQUE INDEX asks_one_open ON asks(kind, tmdb_id) WHERE state IN ('asked', 'approved') AND kind = 'movie';
CREATE INDEX asks_state ON asks(state);
CREATE INDEX asks_title ON asks(title_id);
CREATE INDEX ask_people_user ON ask_people(user_id, asked_at);
CREATE INDEX messages_user ON messages(user_id, id);
