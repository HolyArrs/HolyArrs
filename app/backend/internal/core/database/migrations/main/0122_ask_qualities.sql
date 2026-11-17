-- A movie or show ask remembers which line of "What people can ask for" was
-- picked: its id, the name people saw, and the Pick rules and copy kind it
-- stood for then, so it is approved as asked even if the line changes later.
ALTER TABLE asks ADD COLUMN quality TEXT NOT NULL DEFAULT '';
ALTER TABLE asks ADD COLUMN quality_name TEXT NOT NULL DEFAULT '';
ALTER TABLE asks ADD COLUMN quality_rules_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE asks ADD COLUMN quality_extra INTEGER NOT NULL DEFAULT 0;

-- Older asks were plain or 4K: the Standard line or the 4K line. Their Pick
-- rules stay 0, so approving them follows the Request settings as before.
UPDATE asks SET quality = 'standard' WHERE kind IN ('movie', 'show') AND four_k = 0;
UPDATE asks SET quality = '4k', quality_name = '4K', quality_extra = 1 WHERE kind IN ('movie', 'show') AND four_k = 1;

-- One open ask per movie and chosen line (was: per movie, plain or 4K).
DROP INDEX asks_one_open;
CREATE UNIQUE INDEX asks_one_open ON asks(kind, tmdb_id, quality) WHERE state IN ('asked', 'approved') AND kind = 'movie';
