-- A download's log (Downloads > details) reads its Events by download_id,
-- and the History filters and the clean-up read them by kind and age.
CREATE INDEX IF NOT EXISTS events_download ON events(download_id) WHERE download_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS events_kind_at ON events(kind, at);
