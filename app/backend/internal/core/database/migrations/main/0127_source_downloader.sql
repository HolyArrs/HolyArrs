-- A Source may send its grabs to one chosen Downloader. Empty means any
-- Downloader, as before. No foreign key: a removed Downloader leaves the id
-- behind and the Source then uses any Downloader and says so.
ALTER TABLE sources ADD COLUMN downloader_id INTEGER;
