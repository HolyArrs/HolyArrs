-- new_title_tags: tags a title gets when it is added into this Library
-- folder (a JSON list of labels, empty for none).
ALTER TABLE library_folders ADD COLUMN new_title_tags TEXT NOT NULL DEFAULT '[]';
-- only_tagged_sources: the titles in this folder use only Sources that share
-- one of these tags.
ALTER TABLE library_folders ADD COLUMN only_tagged_sources INTEGER NOT NULL DEFAULT 0;
