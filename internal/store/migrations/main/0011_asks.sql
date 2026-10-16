-- Stage 2.1.11 to 2.1.14: asks for movies (the Wishlist), their approval,
-- per-person limits, in-app messages and Alerts (Discord and email).

-- One ask per movie while it is open (asked or approved); a second person
-- who asks for the same movie joins it in ask_people.
CREATE TABLE asks (
    id            INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind          TEXT NOT NULL DEFAULT 'movie' CHECK (kind IN ('movie')),
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
    available_at  TEXT
);
CREATE UNIQUE INDEX asks_one_open ON asks(kind, tmdb_id) WHERE state IN ('asked', 'approved');
CREATE INDEX asks_state ON asks(state);

-- Everyone who asked: the first person and those who joined. counted says
-- whether the ask counts against the person's weekly limit.
CREATE TABLE ask_people (
    ask_id    INTEGER NOT NULL REFERENCES asks(id) ON DELETE CASCADE,
    user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    asked_at  TEXT NOT NULL,
    counted   INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (ask_id, user_id)
);
CREATE INDEX ask_people_user ON ask_people(user_id, asked_at);

-- Per person: their own weekly limit (NULL takes the default) and where
-- Alerts reach them.
CREATE TABLE user_ask_settings (
    user_id     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    per_week    INTEGER CHECK (per_week IS NULL OR per_week BETWEEN 0 AND 1000),
    email       TEXT NOT NULL DEFAULT '',
    discord_id  TEXT NOT NULL DEFAULT ''
);

-- In-app messages: approved, turned down, ready.
CREATE TABLE messages (
    id          INTEGER PRIMARY KEY,
    created_at  TEXT NOT NULL,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    text        TEXT NOT NULL,
    ask_id      INTEGER REFERENCES asks(id) ON DELETE SET NULL,
    title_id    INTEGER REFERENCES titles(id) ON DELETE SET NULL,
    read_at     TEXT
);
CREATE INDEX messages_user ON messages(user_id, id);

-- Alerts: outgoing messages. Secrets (the Discord link, the mail password)
-- live encrypted in the secrets table.
CREATE TABLE alerts (
    id              INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind            TEXT NOT NULL CHECK (kind IN ('discord', 'email')),
    enabled         INTEGER NOT NULL DEFAULT 1,
    moments         TEXT NOT NULL DEFAULT '[]',
    url_secret_id   INTEGER,
    smtp_host       TEXT NOT NULL DEFAULT '',
    smtp_port       INTEGER NOT NULL DEFAULT 587,
    smtp_security   TEXT NOT NULL DEFAULT 'starttls' CHECK (smtp_security IN ('starttls', 'tls', 'none')),
    smtp_from       TEXT NOT NULL DEFAULT '',
    smtp_user       TEXT NOT NULL DEFAULT '',
    password_secret_id INTEGER,
    staff_to        TEXT NOT NULL DEFAULT '',
    last_sent_at    TEXT,
    last_error      TEXT NOT NULL DEFAULT ''
);
