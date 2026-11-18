-- Why a person is turned off when HolyArrs did it on its own: the kind of
-- media server ("jellyfin" or "emby") their linked account was removed on.
-- Empty when the owner turned them off. When the owner turns them back on it
-- becomes "kept:<kind>", so the check leaves them on until the account is
-- on the server again.
ALTER TABLE users ADD COLUMN off_reason TEXT NOT NULL DEFAULT '';
