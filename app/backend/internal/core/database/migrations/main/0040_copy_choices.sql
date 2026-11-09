-- Existing copies remain edition-unknown and have no audio requirement.
ALTER TABLE versions ADD COLUMN edition TEXT NOT NULL DEFAULT '';
ALTER TABLE versions ADD COLUMN audio_languages TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(audio_languages));
ALTER TABLE files ADD COLUMN protected INTEGER NOT NULL DEFAULT 0 CHECK(protected IN (0, 1));
ALTER TABLE downloads ADD COLUMN confirmed_edition TEXT NOT NULL DEFAULT '';

CREATE TABLE movie_collections (
    id INTEGER PRIMARY KEY,
    source_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);
CREATE TABLE movie_collection_titles (
    collection_id INTEGER NOT NULL REFERENCES movie_collections(id) ON DELETE CASCADE,
    title_id INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
    PRIMARY KEY(collection_id, title_id)
);
CREATE INDEX movie_collection_titles_title ON movie_collection_titles(title_id);
