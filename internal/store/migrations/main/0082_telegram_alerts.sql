-- Roadmap #100: Alerts get a fourth kind, telegram. Its bot token is kept in
-- the vault (under password_secret_id); chat_id is the person, group or
-- channel it writes to. SQLite cannot change a CHECK, so the table is copied.
CREATE TABLE alerts_new (
    id              INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind            TEXT NOT NULL CHECK (kind IN ('discord', 'email', 'webhook', 'telegram')),
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
    last_error      TEXT NOT NULL DEFAULT '',
    allow_local_network INTEGER NOT NULL DEFAULT 0,
    webhook_auth    TEXT NOT NULL DEFAULT 'sign' CHECK (webhook_auth IN ('sign', 'bearer', 'none')),
    chat_id         TEXT NOT NULL DEFAULT ''
);
INSERT INTO alerts_new (id, created_at, created_by, updated_at, updated_by, name, kind, enabled, moments, url_secret_id, smtp_host, smtp_port,
    smtp_security, smtp_from, smtp_user, password_secret_id, staff_to, last_sent_at, last_error, allow_local_network, webhook_auth)
SELECT id, created_at, created_by, updated_at, updated_by, name, kind, enabled, moments, url_secret_id, smtp_host, smtp_port,
    smtp_security, smtp_from, smtp_user, password_secret_id, staff_to, last_sent_at, last_error, allow_local_network, webhook_auth FROM alerts;
DROP TABLE alerts;
ALTER TABLE alerts_new RENAME TO alerts;
