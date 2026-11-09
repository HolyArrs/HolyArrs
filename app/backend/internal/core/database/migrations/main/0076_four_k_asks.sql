-- A movie ask can be for a 4K copy: approving it adds (or keeps) a 4K
-- Version. A 4K ask and a plain ask for the same movie are separate, so
-- the one-open-ask index now includes the flag.
ALTER TABLE asks ADD COLUMN four_k INTEGER NOT NULL DEFAULT 0;
DROP INDEX asks_one_open;
CREATE UNIQUE INDEX asks_one_open ON asks(kind, tmdb_id, four_k) WHERE state IN ('asked', 'approved') AND kind = 'movie';
