-- A title's list of Releases looks up the Releases of its search Jobs and
-- reads job_id, found_by and matched_slot_ids of every Release to find the
-- Jobs; without this it read the whole releases table several times. The
-- index holds those three columns, so that pass never opens the table, and
-- removing a Job no longer reads the table to clear job_id.
CREATE INDEX releases_job ON releases(job_id, found_by, matched_slot_ids);
