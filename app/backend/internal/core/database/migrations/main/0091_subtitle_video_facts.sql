-- Disks that sleep: what the last subtitle look saw of the video, so an
-- unchanged video (same size and modified time, same built-in settings) is
-- not read or hashed again. NULL means "read it again".
ALTER TABLE subtitle_looks ADD COLUMN video_size INTEGER;
ALTER TABLE subtitle_looks ADD COLUMN video_modified INTEGER; -- unix nanoseconds
ALTER TABLE subtitle_looks ADD COLUMN video_read TEXT;        -- built-in settings the tracks were read with
ALTER TABLE subtitle_looks ADD COLUMN video_hash TEXT;        -- the file hash sites match by
