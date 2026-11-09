-- Stable provider identities never match a local account by email or name.
CREATE TABLE sso_links (
    kind         TEXT NOT NULL CHECK (kind IN ('oidc', 'forward_auth')),
    issuer       TEXT NOT NULL,
    subject      TEXT NOT NULL,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    PRIMARY KEY (kind, issuer, subject),
    UNIQUE (user_id, kind, issuer)
);
CREATE INDEX sso_links_user ON sso_links(user_id);

-- Existing sessions have no trustworthy first-factor history.
ALTER TABLE sessions ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE sessions ADD COLUMN local_auth_at TEXT NOT NULL DEFAULT '';
