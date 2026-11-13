-- A leftover is a file in a title's folder that fills no episode (a found
-- note, or a file whose episodes the source no longer lists). The reason
-- is a short key the app translates; dismissed keeps a file the owner
-- wants left alone out of the way across rescans.
ALTER TABLE files ADD COLUMN leftover_reason TEXT;
ALTER TABLE files ADD COLUMN leftover_dismissed INTEGER NOT NULL DEFAULT 0;
