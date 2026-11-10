-- copies_expected = 1 once a person said files copied into this Library
-- folder (no hardlinks from downloads) is how they want it. Health checks
-- then count the folder as fine.
ALTER TABLE library_folders ADD COLUMN copies_expected INTEGER NOT NULL DEFAULT 0;
