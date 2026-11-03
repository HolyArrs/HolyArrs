-- "Clear out finished downloads": once a Usenet download is fully in the
-- Library, HolyArrs removes its finished job and files from SABnzbd or
-- NZBGet. On for every Downloader; the switch is per Downloader.
ALTER TABLE downloaders ADD COLUMN clear_finished INTEGER NOT NULL DEFAULT 1;
