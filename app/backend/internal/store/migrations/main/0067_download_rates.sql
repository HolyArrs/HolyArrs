-- Upload speed, seeds and peers of a torrent download, as its Downloader reports them.
ALTER TABLE downloads ADD COLUMN up_speed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloads ADD COLUMN seeds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE downloads ADD COLUMN peers INTEGER NOT NULL DEFAULT 0;
