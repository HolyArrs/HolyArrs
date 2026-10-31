-- When HolyArrs last saw each torrent in a Downloader's list, written at
-- most once a day per torrent. A daily sweep drops torrent_log and
-- client_links rows for torrents not seen for 30 days.
CREATE TABLE torrent_seen (
	downloader_id INTEGER NOT NULL,
	hash          TEXT NOT NULL,
	seen_at       TEXT NOT NULL,
	PRIMARY KEY (downloader_id, hash)
);
