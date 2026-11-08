-- An episode's still picture URL from the metadata source; empty until the
-- next show refresh.
ALTER TABLE items ADD COLUMN still TEXT NOT NULL DEFAULT '';
