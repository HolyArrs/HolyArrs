-- A movie's directors on their own: people lists the top cast, then the
-- directors, so a screen could not tell them apart. Filled on the next
-- refresh; until then it is empty.
-- The studio is the movie's first production company.
ALTER TABLE titles ADD COLUMN studio TEXT NOT NULL DEFAULT '';
ALTER TABLE titles ADD COLUMN directors TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(directors));
