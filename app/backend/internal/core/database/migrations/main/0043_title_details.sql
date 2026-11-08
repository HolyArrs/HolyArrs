-- Wider artwork, genres, runtime, rating and people for Library titles.
-- Existing titles keep empty values until their next details refresh.
ALTER TABLE titles ADD COLUMN backdrop TEXT NOT NULL DEFAULT '';
ALTER TABLE titles ADD COLUMN logo TEXT NOT NULL DEFAULT '';
ALTER TABLE titles ADD COLUMN genres TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(genres));
ALTER TABLE titles ADD COLUMN runtime_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE titles ADD COLUMN rating TEXT NOT NULL DEFAULT '';
ALTER TABLE titles ADD COLUMN people TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(people));
-- Home reads the newest titles by date added.
CREATE INDEX titles_created ON titles(created_at, id);
