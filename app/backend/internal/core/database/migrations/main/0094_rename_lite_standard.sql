-- The ready-made quality sets get one naming style: Lite, Standard and Best
-- (were Save space, Good everyday and Best). A set someone already named
-- Lite or Standard keeps its name, and the ready-made one keeps the old name.
UPDATE pick_rules SET name = 'Standard'
WHERE name = 'Good everyday' AND preset = 1
  AND NOT EXISTS (SELECT 1 FROM pick_rules WHERE name = 'Standard' COLLATE NOCASE);
UPDATE pick_rules SET name = 'Lite'
WHERE name = 'Save space' AND preset = 1
  AND NOT EXISTS (SELECT 1 FROM pick_rules WHERE name = 'Lite' COLLATE NOCASE);
