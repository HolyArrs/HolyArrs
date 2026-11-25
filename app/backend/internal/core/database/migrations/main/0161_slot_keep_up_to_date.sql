-- One Version may want an episode the others do not: NULL follows the episode and the Version, 1 wants it here anyway, 0 does not.
ALTER TABLE slots ADD COLUMN keep_up_to_date INTEGER;
