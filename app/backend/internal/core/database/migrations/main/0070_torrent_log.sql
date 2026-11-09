-- What happened to each torrent in a Downloader, seen by HolyArrs from now
-- on: state changes and tracker message changes, for HolyArrs downloads and
-- torrents HolyArrs did not send alike. Written only on change; each
-- torrent keeps its newest 200 lines. Tracker lines name the host only.
CREATE TABLE torrent_log (
	id            INTEGER PRIMARY KEY,
	downloader_id INTEGER NOT NULL,
	hash          TEXT NOT NULL,
	at            TEXT NOT NULL,
	kind          TEXT NOT NULL,
	summary       TEXT NOT NULL
);
CREATE INDEX torrent_log_item ON torrent_log (downloader_id, hash, at);
