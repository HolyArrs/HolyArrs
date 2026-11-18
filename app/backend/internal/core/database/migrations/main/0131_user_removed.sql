-- When a person was deleted. Their row stays, renamed and with no way in,
-- so asks and history they left still point at someone.
ALTER TABLE users ADD COLUMN removed_at TEXT NOT NULL DEFAULT '';
-- A deleted person never comes back: nothing may turn their row on again.
CREATE TRIGGER users_removed_stay_off BEFORE UPDATE OF state ON users
WHEN OLD.removed_at != '' AND NEW.state = 'active'
BEGIN
    SELECT RAISE(ABORT, 'a deleted person cannot be turned on');
END;
