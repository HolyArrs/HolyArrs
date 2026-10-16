-- Stage 6.2 review: undo removes a marked row only when it is still the
-- row the move made. SQLite hands a deleted row's id to the next row, so
-- move_made keeps a fingerprint of each row (its created_at and its name
-- or path) and every undo delete requires it. Settings a move changed
-- (naming, subtitle languages) keep the value the move set ("after"), so
-- undo puts the old value back only when nobody changed it since.
-- An undo runs as a Job: undo_state and undo_job_id follow it.
ALTER TABLE move_made ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE move_made ADD COLUMN after TEXT NOT NULL DEFAULT '';
ALTER TABLE moves ADD COLUMN undo_state TEXT NOT NULL DEFAULT '' CHECK (undo_state IN ('', 'queued', 'running', 'failed'));
ALTER TABLE moves ADD COLUMN undo_job_id INTEGER;

-- Backfill: a marked row made within 10 seconds of its mark is the row the
-- move made. Any other mark keeps an empty fingerprint, and undo keeps
-- that row ("cannot verify").
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM sources t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'source';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM downloaders t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'downloader';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM alerts t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'alert';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM users t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'user';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM asks t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'ask';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM never_add t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'never_add';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.name FROM subtitle_sites t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'subtitle_site';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.kind FROM titles t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'title';
UPDATE move_made SET fingerprint = COALESCE((SELECT t.created_at || char(31) || t.path FROM library_folders t
    WHERE t.id = move_made.row_id AND abs(julianday(t.created_at) - julianday(move_made.created_at)) * 86400 < 10), '') WHERE what = 'folder';
