-- Stage 5.1: webhook Alerts (5.1.11), the health checks (5.1.13) and the
-- media server link (5.1.14).

-- Alerts get a third kind, webhook. Its secret (in the vault, under
-- password_secret_id) either signs each message or is sent as a bearer
-- token, as webhook_auth says.
CREATE TABLE alerts_new (
    id              INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind            TEXT NOT NULL CHECK (kind IN ('discord', 'email', 'webhook')),
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
    webhook_auth    TEXT NOT NULL DEFAULT 'sign' CHECK (webhook_auth IN ('sign', 'bearer', 'none'))
);
INSERT INTO alerts_new (id, created_at, created_by, updated_at, updated_by, name, kind, enabled, moments, url_secret_id, smtp_host, smtp_port,
    smtp_security, smtp_from, smtp_user, password_secret_id, staff_to, last_sent_at, last_error, allow_local_network)
SELECT id, created_at, created_by, updated_at, updated_by, name, kind, enabled, moments, url_secret_id, smtp_host, smtp_port,
    smtp_security, smtp_from, smtp_user, password_secret_id, staff_to, last_sent_at, last_error, allow_local_network FROM alerts;
DROP TABLE alerts;
ALTER TABLE alerts_new RENAME TO alerts;

-- One row per health check and what it checks (check_key such as
-- downloader:3 or backups). state is the latest result; confirmed is the
-- result Alerts were told about, which follows state only after it held
-- for streak runs in a row (the flap guard).
CREATE TABLE health_checks (
    id           INTEGER PRIMARY KEY,
    check_key    TEXT NOT NULL UNIQUE,
    kind         TEXT NOT NULL,
    subject      TEXT NOT NULL DEFAULT '',
    state        TEXT NOT NULL CHECK (state IN ('ok', 'warning', 'error')),
    message      TEXT NOT NULL DEFAULT '',
    since        TEXT NOT NULL,
    checked_at   TEXT NOT NULL,
    streak       INTEGER NOT NULL DEFAULT 1,
    confirmed    TEXT NOT NULL DEFAULT 'ok' CHECK (confirmed IN ('ok', 'warning', 'error'))
);

-- Run the health checks every 5 minutes.
INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'health_check', 300, 1)
ON CONFLICT (kind) DO NOTHING;

-- Media servers HolyArrs tells to look at a movie folder again after an
-- import, a rename or a delete. The token is in the vault.
CREATE TABLE media_servers (
    id              INTEGER PRIMARY KEY,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    name            TEXT NOT NULL UNIQUE COLLATE NOCASE,
    kind            TEXT NOT NULL CHECK (kind IN ('plex', 'jellyfin', 'emby')),
    url             TEXT NOT NULL,
    token_secret_id INTEGER,
    enabled         INTEGER NOT NULL DEFAULT 1,
    on_import       INTEGER NOT NULL DEFAULT 1,
    on_rename       INTEGER NOT NULL DEFAULT 1,
    on_delete       INTEGER NOT NULL DEFAULT 1,
    path_from       TEXT NOT NULL DEFAULT '',
    path_to         TEXT NOT NULL DEFAULT '',
    allow_local_network INTEGER NOT NULL DEFAULT 1,
    last_ok_at      TEXT,
    last_error      TEXT NOT NULL DEFAULT ''
);
