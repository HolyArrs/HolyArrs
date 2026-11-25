-- stand_in = 1: not a real folder. In hands-off mode a title added while no
-- Library folder of its kind exists is kept under this row, so its copy has
-- a folder to point at. Nothing is ever read from or written to its path,
-- and it is left out of every folder list and check.
ALTER TABLE library_folders ADD COLUMN stand_in INTEGER NOT NULL DEFAULT 0;
