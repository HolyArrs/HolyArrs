-- Artist packages copied from the metadata server (internal/musicsync): a
-- music page answers from these rows, never waiting on the server.
CREATE TABLE music_packages (
	mbid TEXT PRIMARY KEY,
	version INTEGER NOT NULL,
	hash TEXT NOT NULL,
	rank INTEGER NOT NULL DEFAULT 0,
	complete INTEGER NOT NULL DEFAULT 0,
	body BLOB NOT NULL,
	-- pictures: the artist's picture records (JSON), read at start.
	pictures TEXT NOT NULL DEFAULT '{}',
	stored_at TEXT NOT NULL
);
-- Which package holds each release group (album pages, album artist).
CREATE TABLE music_package_albums (
	rg TEXT PRIMARY KEY,
	artist TEXT NOT NULL,
	-- cover: the cover's picture record (JSON), '' when none is stored.
	cover TEXT NOT NULL DEFAULT ''
);
CREATE INDEX music_package_albums_artist ON music_package_albums (artist);
-- The sync cursor: the server's package version copied up to.
CREATE TABLE music_package_sync (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	cursor INTEGER NOT NULL,
	synced_at TEXT NOT NULL
);
