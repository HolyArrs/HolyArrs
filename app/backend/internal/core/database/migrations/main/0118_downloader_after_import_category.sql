-- A qBittorrent Downloader may move a torrent to another category once it is
-- fully imported (category_after_import, '' keeps it where it is), and may
-- show only HolyArrs's own items (own_only, off by default). A Download
-- records when it was moved (category_moved_at), so it moves once. Add-only.
ALTER TABLE downloaders ADD COLUMN category_after_import TEXT NOT NULL DEFAULT '';
ALTER TABLE downloaders ADD COLUMN own_only INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloads ADD COLUMN category_moved_at TEXT;
