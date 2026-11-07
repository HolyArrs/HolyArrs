-- Future seasons only: a show keeps wanting only episodes that air after
-- future_from (set when Future seasons only is picked), plus new seasons.
-- An ask may ask for the future seasons only (future_only = 1).
ALTER TABLE titles ADD COLUMN future_from TEXT;
ALTER TABLE asks ADD COLUMN future_only INTEGER NOT NULL DEFAULT 0;
