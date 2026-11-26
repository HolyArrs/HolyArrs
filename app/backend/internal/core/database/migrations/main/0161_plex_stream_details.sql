ALTER TABLE plex_playbacks ADD COLUMN stream_details TEXT;
ALTER TABLE plex_playbacks ADD COLUMN plex_thumb TEXT NOT NULL DEFAULT '';
ALTER TABLE plex_playbacks ADD COLUMN source_part TEXT NOT NULL DEFAULT '';
ALTER TABLE plex_playbacks ADD COLUMN file_id INTEGER REFERENCES files(id) ON DELETE SET NULL;
ALTER TABLE plex_playbacks ADD COLUMN version_id INTEGER REFERENCES versions(id) ON DELETE SET NULL;
CREATE INDEX plex_playbacks_file ON plex_playbacks(file_id);
CREATE INDEX plex_playbacks_version ON plex_playbacks(version_id);
