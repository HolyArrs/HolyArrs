-- Own traits synced from TRaSH Guides (#172). A synced trait keeps where it
-- came from (source "trash", source_id "<app>:<trash id>") so a later sync
-- updates it in place, and holds its tests in specs: every test kind must
-- pass, like a release format in the old apps.
ALTER TABLE own_traits ADD COLUMN source TEXT NOT NULL DEFAULT '';
ALTER TABLE own_traits ADD COLUMN source_id TEXT NOT NULL DEFAULT '';
ALTER TABLE own_traits ADD COLUMN specs TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(specs));
CREATE UNIQUE INDEX own_traits_source ON own_traits(source, source_id) WHERE source != '';
