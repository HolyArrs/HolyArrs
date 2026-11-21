-- IMDb, Rotten Tomatoes and Metacritic scores, read from OMDb with the
-- owner's own OMDb key. NULL means not known; imdb_votes is 0 then.
-- omdb_checked_at is when OMDb was last asked, so a title is asked again
-- at most once a week (the free key allows 1,000 requests a day).
ALTER TABLE titles ADD COLUMN imdb_rating REAL;
ALTER TABLE titles ADD COLUMN imdb_votes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE titles ADD COLUMN rotten_tomatoes INTEGER;
ALTER TABLE titles ADD COLUMN metacritic INTEGER;
ALTER TABLE titles ADD COLUMN omdb_checked_at TEXT;
