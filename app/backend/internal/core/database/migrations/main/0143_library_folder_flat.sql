-- flat = 1: a movie Library folder that keeps every movie as a file
-- directly in it, no folder per movie. New movies land there as
-- files; their Version folder_name is ''.
ALTER TABLE library_folders ADD COLUMN flat INTEGER NOT NULL DEFAULT 0;
