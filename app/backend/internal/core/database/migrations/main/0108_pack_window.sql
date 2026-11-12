-- A show can take a season pack a few episodes at a time: pack_window is how
-- many episodes ahead the Downloader fetches from the pack; 0 is off, so the
-- whole season downloads as before.
ALTER TABLE titles ADD COLUMN pack_window INTEGER NOT NULL DEFAULT 0;
-- A download that fetches only some files of a pack keeps them here as JSON
-- (each file's index and name, the Slots it fills, and whether it is
-- switched on). NULL is a normal download that fetches everything.
ALTER TABLE downloads ADD COLUMN pack_files TEXT;
