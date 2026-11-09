-- "HolyArrs can change this Downloader": on for every Downloader; a person
-- turns it off in Settings > Downloaders. Off, HolyArrs still sends new
-- downloads but never changes or removes what is already in the client.
ALTER TABLE downloaders ADD COLUMN writable INTEGER NOT NULL DEFAULT 1;
