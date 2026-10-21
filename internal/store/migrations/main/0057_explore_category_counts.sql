-- The Explore category strip's own counts, per member, category and day:
-- how often a category tile was shown and opened. Unlike explore_counts,
-- these do reorder: the strip learns from them (smoothed open rate, skips
-- fading over 14 days, the last 30 days read). Kept 90 days, local only,
-- cleared by the member's Reset my recommendations and with the member.
-- Add-only.
CREATE TABLE explore_category_counts (
    member_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id TEXT NOT NULL,
    day         TEXT NOT NULL,
    shown       INTEGER NOT NULL DEFAULT 0,
    opened      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (member_id, category_id, day)
);
CREATE INDEX explore_category_counts_day ON explore_category_counts(day);
