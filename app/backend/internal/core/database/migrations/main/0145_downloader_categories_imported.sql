-- categories_imported: the categories an import from the old apps set on
-- this Downloader. HolyArrs may make one of these in the client on a check
-- when it is missing there (and the Downloader may be changed).
ALTER TABLE downloaders ADD COLUMN categories_imported TEXT NOT NULL DEFAULT '[]';
