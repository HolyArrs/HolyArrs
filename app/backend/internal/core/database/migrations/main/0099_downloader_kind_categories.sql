-- A Downloader may use its own category for movies, shows and music. Empty uses the main category.
ALTER TABLE downloaders ADD COLUMN category_movies TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN category_shows TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN category_music TEXT NOT NULL DEFAULT '';
