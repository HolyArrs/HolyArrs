-- Whether an episode's air_date is its exact air time (1) or only stands in
-- for its air day (0: the day's end in UTC, so it never counts as aired
-- early). Set by the details source at each refresh. Rows written before
-- this column: an air_date equal to the end of its air day was filled in
-- from the day alone and is untimed; any other air_date is a real time.
ALTER TABLE items ADD COLUMN air_timed INTEGER NOT NULL DEFAULT 0 CHECK (air_timed IN (0, 1));
UPDATE items SET air_timed = 1
WHERE kind = 'episode' AND air_date IS NOT NULL
  AND (air_day IS NULL OR substr(air_date, 1, 19) != date(air_day, '+1 day') || 'T00:00:00');
