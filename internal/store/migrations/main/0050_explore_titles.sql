-- Explore row titles: which title template each member saw on which row
-- and day, so a template rests 14 days before it shows again and a row's
-- title holds for the whole day. Kept 30 days.
CREATE TABLE explore_titles (
    member_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day         TEXT NOT NULL,
    row_id      TEXT NOT NULL,
    template_id TEXT NOT NULL,
    PRIMARY KEY (member_id, day, row_id)
);
CREATE INDEX explore_titles_member_day ON explore_titles (member_id, day);
-- Counts kept to compare smart titles with plain ones, by title layer,
-- never to reorder Explore. Kept 90 days, shown to the owner only.
CREATE TABLE explore_title_counts (
    member_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    layer     TEXT NOT NULL,
    day       TEXT NOT NULL,
    shown     INTEGER NOT NULL DEFAULT 0,
    opens     INTEGER NOT NULL DEFAULT 0,
    asks      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (member_id, layer, day)
);
