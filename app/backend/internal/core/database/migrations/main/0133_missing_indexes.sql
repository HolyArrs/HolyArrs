-- A foreign key needs an index on its child column. Without one, every
-- deleted parent row scans the whole child table. Deleting a Source
-- deletes its releases, and each release then scanned every slot and
-- download: 20,000 releases over 60,000 slots took over 7 minutes and was
-- rolled back at the write cap. With these it takes under a second.
-- Each index is on a table that points at sources, releases, downloads,
-- titles, items, parts, versions, slots or files, the tables that grow with
-- the Library.
CREATE INDEX slots_chosen_release ON slots(chosen_release_id);
CREATE INDEX slots_held_release ON slots(held_release_id);
CREATE INDEX slots_active_download ON slots(active_download_id);
CREATE INDEX slots_file ON slots(file_id);
CREATE INDEX slots_version ON slots(version_id);
CREATE INDEX downloads_release ON downloads(release_id);
CREATE INDEX files_parent ON files(parent_file_id);
CREATE INDEX rename_steps_file ON rename_steps(file_id);
CREATE INDEX subtitle_looks_file ON subtitle_looks(file_id);
CREATE INDEX subtitle_wishes_item ON subtitle_wishes(item_id);
CREATE INDEX subtitle_wishes_version ON subtitle_wishes(version_id);
CREATE INDEX reports_file ON reports(file_id);
CREATE INDEX reports_version ON reports(version_id);
CREATE INDEX reports_item ON reports(item_id);
CREATE INDEX reports_album ON reports(album_id);
CREATE INDEX media_labels_title ON media_labels(title_id);
CREATE INDEX messages_title ON messages(title_id);
CREATE INDEX new_album_notices_title ON new_album_notices(title_id);
CREATE INDEX new_episode_notices_title ON new_episode_notices(title_id);

-- Titles are found by their outside ids on every add, import, request and
-- old-app call, and each such lookup read every title. A query uses one of
-- these only when it names the same path as a literal, as in
-- json_extract(source_ids, '$.tmdb') = ?.
-- Ids SQLite reads only as JSON5 (unquoted keys, a trailing comma) are
-- rewritten as plain JSON, keeping every id. Only text it cannot read at
-- all, which would make building the indexes fail and makes every lookup
-- that reads it fail already, becomes empty.
UPDATE titles SET source_ids = CASE WHEN json_valid(source_ids, 2) THEN json(source_ids) ELSE '{}' END
    WHERE json_valid(source_ids, 1) = 0;
CREATE INDEX titles_tmdb ON titles(json_extract(source_ids, '$.tmdb'));
CREATE INDEX titles_tvdb ON titles(json_extract(source_ids, '$.tvdb'));
CREATE INDEX titles_imdb ON titles(json_extract(source_ids, '$.imdb'));
CREATE INDEX titles_tvmaze ON titles(json_extract(source_ids, '$.tvmaze'));
CREATE INDEX titles_musicbrainz ON titles(json_extract(source_ids, '$.musicbrainz'));
