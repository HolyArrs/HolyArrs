-- The picture each wide title card shows, chosen once per title and
-- language by the Explore pool job and kept 30 days: art is the backdrop,
-- logo the title logo (empty when none exists; kept with titled art for
-- the grown card over the trailer), has_title says the backdrop already shows the title.
CREATE TABLE card_art (
    kind       TEXT NOT NULL,
    tmdb_id    TEXT NOT NULL,
    lang       TEXT NOT NULL,
    art        TEXT NOT NULL DEFAULT '',
    logo       TEXT NOT NULL DEFAULT '',
    has_title  INTEGER NOT NULL DEFAULT 0,
    fetched_at TEXT NOT NULL,
    PRIMARY KEY (kind, tmdb_id, lang)
);
