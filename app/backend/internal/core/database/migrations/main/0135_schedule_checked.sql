-- A scheduled check that found nothing to do is kept, with why, so the
-- Schedules page and a restart both know it. last_run still only
-- moves when a Job is queued.
ALTER TABLE schedules ADD COLUMN last_checked TEXT;
ALTER TABLE schedules ADD COLUMN last_skip TEXT;
