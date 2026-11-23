-- People who linked a plugin (a chat bot, a list sync) to their own account:
-- that plugin may then make requests and follow titles for them, with their
-- own rights and limits.

CREATE TABLE plugin_links (
    plugin_id  TEXT NOT NULL,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    external   TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (plugin_id, user_id)
);
