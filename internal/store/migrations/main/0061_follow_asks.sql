-- Follow: asking for a title follows it (shows its dates on the asker's
-- calendar), unless the asker unticks Follow. The choice is kept per asker
-- so the follow can start once the ask is approved and the title is added.
-- People already follow the Library titles they asked for. Movies can be
-- followed like shows and artists from now on (no table change needed).
ALTER TABLE ask_people ADD COLUMN follow INTEGER NOT NULL DEFAULT 1;
INSERT OR IGNORE INTO follows (user_id, title_id, created_at)
    SELECT p.user_id, a.title_id, min(p.asked_at) FROM ask_people p
    JOIN asks a ON a.id = p.ask_id JOIN titles t ON t.id = a.title_id
    JOIN users u ON u.id = p.user_id
    WHERE a.title_id IS NOT NULL AND a.state IN ('approved', 'available') AND t.state != 'removed'
    GROUP BY p.user_id, a.title_id;
