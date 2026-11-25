-- An invite link can take more than one person. max_uses is how many may
-- join through it (0 for no limit) and uses is how many have. used_at stays
-- the mark of a link that takes no one else: full, or ended by its maker.
ALTER TABLE invites ADD COLUMN max_uses INTEGER NOT NULL DEFAULT 1;
ALTER TABLE invites ADD COLUMN uses INTEGER NOT NULL DEFAULT 0;
UPDATE invites SET uses = 1 WHERE used_at IS NOT NULL;

-- Everyone who joined through a link, so the list can name them all.
CREATE TABLE invite_uses (
    hash     TEXT NOT NULL REFERENCES invites(hash) ON DELETE CASCADE,
    user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_at  TEXT NOT NULL,
    PRIMARY KEY (hash, user_id)
);
INSERT INTO invite_uses(hash, user_id, used_at)
    SELECT hash, used_by, used_at FROM invites WHERE used_at IS NOT NULL AND used_by IS NOT NULL;
