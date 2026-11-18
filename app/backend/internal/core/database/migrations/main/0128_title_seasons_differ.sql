-- A show imported from an old app that numbers its seasons differently from
-- the show here: a file found in its folder is never put on an episode by
-- the numbers in its name; the owner matches each one by hand.
ALTER TABLE titles ADD COLUMN seasons_differ INTEGER NOT NULL DEFAULT 0;
