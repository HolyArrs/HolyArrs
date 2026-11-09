-- 3.5.9 review: a secret that is changed or removed must stay masked in the
-- log files after it leaves the vault. The old value is kept here, still
-- sealed exactly as it was in secrets (never in the clear), until the log
-- masker has rewritten the log files with it masked; then the row goes.
CREATE TABLE secrets_retired (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    secret_id INTEGER NOT NULL,
    value     TEXT NOT NULL
);

CREATE TRIGGER secrets_retired_on_delete AFTER DELETE ON secrets
WHEN OLD.value <> '' AND OLD.kind <> 'check'
BEGIN
    INSERT INTO secrets_retired(secret_id, value) VALUES (OLD.id, OLD.value);
END;

CREATE TRIGGER secrets_retired_on_update AFTER UPDATE OF value ON secrets
WHEN OLD.value <> '' AND OLD.value <> NEW.value AND OLD.kind <> 'check'
BEGIN
    INSERT INTO secrets_retired(secret_id, value) VALUES (OLD.id, OLD.value);
END;
