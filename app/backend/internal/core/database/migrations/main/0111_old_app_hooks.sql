-- The old-app API keeps two more things for the apps that call it.
--
-- compat_hooks: the webhook records a request app asks a side to keep, so
-- HolyArrs tells it when a file was imported. The record is kept as sent,
-- without its address, headers and sign-in, which sit in one sealed
-- secret (secret_id). on_import and on_upgrade are the two switches the
-- sender reads; tags is the JSON list of tag ids the record is limited to.
-- AUTOINCREMENT so a removed record's id is never handed out again.
CREATE TABLE compat_hooks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    side       TEXT NOT NULL CHECK (side IN ('sonarr', 'radarr')),
    name       TEXT NOT NULL COLLATE NOCASE,
    method     TEXT NOT NULL DEFAULT 'POST' CHECK (method IN ('POST', 'PUT')),
    on_import  INTEGER NOT NULL DEFAULT 0,
    on_upgrade INTEGER NOT NULL DEFAULT 0,
    tags       TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(tags)),
    body       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(body)),
    secret_id  INTEGER,
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    UNIQUE (side, name)
);

-- compat_title_tags: which titles carry which of those apps' tags.
CREATE TABLE compat_title_tags (
    tag_id   INTEGER NOT NULL REFERENCES compat_tags(id) ON DELETE CASCADE,
    title_id INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    PRIMARY KEY (tag_id, title_id)
);
CREATE INDEX compat_title_tags_title ON compat_title_tags(title_id);
