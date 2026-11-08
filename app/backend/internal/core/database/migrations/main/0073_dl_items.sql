-- The background download reader (downloads.sync = on) keeps each client's
-- last known items here, so a restart shows them at once and the torrent
-- log carries on across it. Written only on change, in batches; never
-- read or written with downloads.sync = legacy. One row per item in one
-- Downloader; data holds the whole item as last read.
CREATE TABLE dl_items (
	downloader_id  INTEGER NOT NULL,
	key            TEXT NOT NULL,
	name           TEXT NOT NULL DEFAULT '',
	state          TEXT NOT NULL DEFAULT '',
	phase          TEXT NOT NULL DEFAULT '',
	size           INTEGER NOT NULL DEFAULT 0,
	done_bytes     INTEGER NOT NULL DEFAULT 0,
	uploaded       INTEGER NOT NULL DEFAULT 0,
	ratio          REAL NOT NULL DEFAULT 0,
	category       TEXT NOT NULL DEFAULT '',
	save_path      TEXT NOT NULL DEFAULT '',
	added_at       INTEGER NOT NULL DEFAULT 0,
	completed_at   INTEGER NOT NULL DEFAULT 0,
	last_change_at TEXT NOT NULL DEFAULT '',
	gone_at        TEXT,
	data           TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (downloader_id, key)
);
-- Client items with no HolyArrs download wait here for a title match.
-- Matching links only: it never adds to the Library. outcome is '' while
-- waiting, then linked, suggested, unsure, not_media, own or nomatch.
CREATE TABLE dl_match_queue (
	downloader_id   INTEGER NOT NULL,
	key             TEXT NOT NULL,
	name            TEXT NOT NULL DEFAULT '',
	category        TEXT NOT NULL DEFAULT '',
	priority        INTEGER NOT NULL DEFAULT 3,
	attempts        INTEGER NOT NULL DEFAULT 0,
	next_try_at     TEXT NOT NULL DEFAULT '',
	last_error      TEXT NOT NULL DEFAULT '',
	outcome         TEXT NOT NULL DEFAULT '',
	linked_title_id INTEGER,
	PRIMARY KEY (downloader_id, key)
);
CREATE INDEX dl_match_queue_due ON dl_match_queue (outcome, next_try_at);
