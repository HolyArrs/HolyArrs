-- The first-run wizard now keeps the owner in it until it is finished
-- (first_run_done). Installs that already have accounts finished their
-- setup before this flag existed, so they are marked done.
INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
SELECT 'this_app', 'first_run_done', '1', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}'
WHERE EXISTS (SELECT 1 FROM users)
ON CONFLICT(grp, key) DO NOTHING;
