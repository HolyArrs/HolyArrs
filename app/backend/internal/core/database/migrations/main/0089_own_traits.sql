-- Your own traits (#97): a named test on a Release's name or its release
-- group, used in Pick rules like any other trait (key "own:<id>").
CREATE TABLE own_traits (
  id INTEGER PRIMARY KEY AUTOINCREMENT, -- a removed trait's id is never given again
  created_at TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(created_by)),
  updated_at TEXT NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(updated_by)),
  name TEXT NOT NULL,
  match TEXT NOT NULL CHECK(match IN ('name', 'group')),
  terms TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(terms))
);
CREATE UNIQUE INDEX own_traits_name ON own_traits(lower(name));
