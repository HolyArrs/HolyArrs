-- A finished Usenet job whose clear from its Downloader failed waits here
-- for its next try (clear_retry_at), with how many tries ran and the last
-- reason. NULL clear_retry_at: nothing waiting. Add-only.
ALTER TABLE downloads ADD COLUMN clear_retry_at TEXT;
ALTER TABLE downloads ADD COLUMN clear_tries INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloads ADD COLUMN clear_error TEXT NOT NULL DEFAULT '';
