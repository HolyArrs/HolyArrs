-- Preserve the existing single destination unless per-type mode is selected.
ALTER TABLE downloaders ADD COLUMN after_import_per_type INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloaders ADD COLUMN category_after_import_movies TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN category_after_import_shows TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN category_after_import_music TEXT NOT NULL DEFAULT '';
