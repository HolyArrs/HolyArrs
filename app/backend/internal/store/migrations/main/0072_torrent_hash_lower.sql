-- Torrent hashes are stored lower-case from now on, so removals and the
-- daily sweep match them with a plain indexed comparison. Older rows are
-- lower-cased here; a rare case-only duplicate keeps its old spelling and
-- leaves with the daily sweep by its age.
UPDATE OR IGNORE torrent_seen SET hash = lower(hash) WHERE hash <> lower(hash);
UPDATE OR IGNORE client_links SET hash = lower(hash) WHERE hash <> lower(hash);
UPDATE torrent_log SET hash = lower(hash) WHERE hash <> lower(hash);
-- (downloader_id, hash) lookups: torrent_seen uses its primary key,
-- client_links its unique constraint, torrent_log this index.
CREATE INDEX IF NOT EXISTS torrent_log_item ON torrent_log (downloader_id, hash, at);
