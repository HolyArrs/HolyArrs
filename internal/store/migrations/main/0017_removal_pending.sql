-- Stage 4.1.3: a Remove that could not reach its Downloader is recorded
-- here (1 = delete its files too, 0 = keep them) and tried again on start
-- and on each sync until the Downloader confirms. Add-only.
ALTER TABLE downloads ADD COLUMN removal_pending INTEGER;
