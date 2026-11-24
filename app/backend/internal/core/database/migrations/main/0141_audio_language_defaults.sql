-- The audio languages a release carries on a Source when its name shows none
-- (a tracker whose rules say every release has them). Empty keeps the old
-- reading: no language word means the title's original language.
ALTER TABLE sources ADD COLUMN no_language_languages TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(no_language_languages));
-- The audio languages the main copy of each new movie or show added into a
-- Library folder needs. Empty: any audio.
ALTER TABLE library_folders ADD COLUMN new_title_audio TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(new_title_audio));
