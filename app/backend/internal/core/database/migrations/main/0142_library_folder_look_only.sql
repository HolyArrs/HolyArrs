-- look_only = 1: HolyArrs only reads this Library folder. Its titles show as
-- in the Library, but nothing is downloaded, renamed, moved, tidied, written
-- or deleted there (#419).
ALTER TABLE library_folders ADD COLUMN look_only INTEGER NOT NULL DEFAULT 0;
