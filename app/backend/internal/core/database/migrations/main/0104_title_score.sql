-- A movie's or show's members' average out of 10 and its vote count, so the
-- Library can sort by score. A title from before this has no score (NULL)
-- until its next details refresh fills it; 0 means looked up with none.
ALTER TABLE titles ADD COLUMN score REAL;
ALTER TABLE titles ADD COLUMN votes INTEGER NOT NULL DEFAULT 0;
