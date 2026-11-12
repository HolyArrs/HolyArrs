-- A Version is the title's 4K copy by a stored mark, not by its
-- label or its Pick rules' name, so renaming either keeps it the 4K copy.
-- A title has at most one 4K copy.
ALTER TABLE versions ADD COLUMN four_k INTEGER NOT NULL DEFAULT 0;

-- Old 4K copies get the mark, the oldest one per title: a second copy
-- labelled 4K or on Pick rules named 4K.
UPDATE versions SET four_k = 1 WHERE id IN (
	SELECT min(v.id) FROM versions v LEFT JOIN pick_rules p ON p.id = v.pick_rules_id
	WHERE v.is_main = 0 AND (v.label = '4K' COLLATE NOCASE OR p.name = '4K')
	GROUP BY v.title_id);

-- A main copy on the 4K Pick rules is the 4K copy only when a 4K ask
-- added it (a Library that picks 4K for every title keeps plain copies).
UPDATE versions SET four_k = 1 WHERE is_main = 1
	AND pick_rules_id IN (SELECT id FROM pick_rules WHERE name = '4K')
	AND title_id IN (SELECT title_id FROM asks WHERE four_k = 1 AND title_id IS NOT NULL)
	AND title_id NOT IN (SELECT title_id FROM versions WHERE four_k = 1);

CREATE UNIQUE INDEX versions_one_four_k ON versions(title_id) WHERE four_k = 1;
