-- Roadmap #104: an Outside list can hold shows as well as movies. title_kind
-- says which; a show list keeps its own season choice (seasons: all, future,
-- latest, first, none) and what happens to seasons that come out later
-- (new_seasons: all, none). Movie lists leave both empty.
ALTER TABLE outside_lists ADD COLUMN title_kind TEXT NOT NULL DEFAULT 'movie' CHECK (title_kind IN ('movie', 'show'));
ALTER TABLE outside_lists ADD COLUMN seasons TEXT NOT NULL DEFAULT '';
ALTER TABLE outside_lists ADD COLUMN new_seasons TEXT NOT NULL DEFAULT '';
