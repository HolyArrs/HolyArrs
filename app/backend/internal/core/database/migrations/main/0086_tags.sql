-- Tags (#108): plain lowercase labels on titles, Downloaders and Sources.
-- A title's tags route its grabs and searches to the Downloaders and
-- Sources that share one of them.
ALTER TABLE titles ADD COLUMN tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags));
ALTER TABLE downloaders ADD COLUMN tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags));
ALTER TABLE sources ADD COLUMN tags TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tags));
