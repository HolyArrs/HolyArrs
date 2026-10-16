-- 4.15 review: how exact an album's release date is (day, month or year).
-- A date known only by month or year is stored at the end of it (the
-- release gate), and has no release day for the Calendar. Albums stored
-- before this are NULL until their artist is refreshed, and count as day.
ALTER TABLE parts ADD COLUMN release_date_precision TEXT CHECK (release_date_precision IN ('day', 'month', 'year'));
