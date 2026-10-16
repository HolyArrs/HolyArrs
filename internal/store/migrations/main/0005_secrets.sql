-- Encrypted secrets (design/domain/model.md, Settings). value is
-- "v1:" + base64(nonce || AES-256-GCM ciphertext); the row id is bound in as
-- extra data, so a value cannot be moved to another row. The master key lives
-- outside the database, in the config folder or a file named by
-- HOLYARRS_MASTER_KEY_FILE, and is never part of a backup.
CREATE TABLE secrets (
    id          INTEGER PRIMARY KEY,
    created_at  TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at  TEXT NOT NULL, updated_by TEXT NOT NULL,
    kind        TEXT NOT NULL,
    value       TEXT NOT NULL
);
