-- A show ask may ask for single episodes of a season (JSON list of
-- {season, episodes}) besides whole seasons. Empty: none.
ALTER TABLE asks ADD COLUMN episodes TEXT NOT NULL DEFAULT '[]';
