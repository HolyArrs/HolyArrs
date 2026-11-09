-- Torrents found in a Downloader that HolyArrs did not send, matched to a
-- Library title. A link only names the title (and its episode slots) for
-- the Downloads screen; it is never a download record, so the sync,
-- failure and import logic never acts on it. state: linked, unsure,
-- not_media, skipped. candidates keeps the looked-up choices (JSON) so a
-- lookup is never repeated.
CREATE TABLE client_links (
	id            INTEGER PRIMARY KEY,
	downloader_id INTEGER NOT NULL,
	hash          TEXT NOT NULL,
	name          TEXT NOT NULL DEFAULT '',
	state         TEXT NOT NULL,
	title_id      INTEGER,
	slot_ids      TEXT NOT NULL DEFAULT '[]',
	covers        TEXT NOT NULL DEFAULT '',
	candidates    TEXT NOT NULL DEFAULT '[]',
	how           TEXT NOT NULL DEFAULT '',
	matched_at    TEXT NOT NULL,
	UNIQUE (downloader_id, hash)
);
