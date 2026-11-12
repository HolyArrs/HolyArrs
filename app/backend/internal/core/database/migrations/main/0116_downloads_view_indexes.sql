-- The Downloads view looks up each Download's Imports and the Downloads
-- waiting to be cleared on every refresh; without these it read both
-- whole tables. The second index only holds rows that are waiting.
CREATE INDEX imports_download ON imports(download_id);
CREATE INDEX downloads_clear_retry ON downloads(clear_retry_at) WHERE clear_retry_at IS NOT NULL;
