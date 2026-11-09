-- Explore: each member's scored candidate rows per type switch and day,
-- built by a background job. stale marks a pool to rebuild (an ask by the
-- member, or a title added); due is when the rebuild may start (adds wait
-- 10 minutes after the last one). Old dates are pruned after 2 days.
CREATE TABLE explore_pool (
    member_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type_switch TEXT NOT NULL CHECK (type_switch IN ('all', 'movies', 'shows')),
    day         TEXT NOT NULL,
    built_at    TEXT NOT NULL,
    stale       INTEGER NOT NULL DEFAULT 0,
    due         TEXT,
    pool        TEXT NOT NULL CHECK (json_valid(pool)),
    PRIMARY KEY (member_id, type_switch, day)
);
-- The streaming services and region a member's pools are built for.
CREATE TABLE explore_prefs (
    member_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    services  TEXT NOT NULL DEFAULT '',
    region    TEXT NOT NULL DEFAULT ''
);
-- Counts kept to measure Explore, never to reorder it: per member, row
-- type and day. Kept 90 days, shown to the owner only. (The category
-- strip may reorder from its own counts, explore_category_counts, 0057.)
CREATE TABLE explore_counts (
    member_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    row_type  TEXT NOT NULL,
    day       TEXT NOT NULL,
    shown     INTEGER NOT NULL DEFAULT 0,
    opens     INTEGER NOT NULL DEFAULT 0,
    asks      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (member_id, row_type, day)
);
