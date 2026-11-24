-- When the download was marked cleaned up (removed). The Downloads list leaves
-- such rows out, and the row keeps hiding the Downloader's copy of the job for
-- a short while after this time (see removedKeepsListing). It is its own
-- column, not updated_at, so a later write to the row does not restart that
-- wait. Rows cleaned before this column existed take the last write they had.
ALTER TABLE downloads ADD COLUMN cleaned_at TEXT;
UPDATE downloads SET cleaned_at = updated_at WHERE state = 'cleaned';
