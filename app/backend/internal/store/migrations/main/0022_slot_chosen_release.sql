-- Stage 3.2 review: a chosen Slot names the Release it was chosen for, so a
-- season pack grabbed for one Slot pulls in only the other Slots chosen for
-- that same Release (an old pick list can name Slots chosen since for
-- another one).

ALTER TABLE slots ADD COLUMN chosen_release_id INTEGER REFERENCES releases(id) ON DELETE SET NULL;

UPDATE slots SET chosen_release_id = (
    SELECT r.id FROM releases r
    WHERE r.state = 'chosen' AND EXISTS (SELECT 1 FROM json_each(r.matched_slot_ids) WHERE value = slots.id)
    ORDER BY r.id DESC LIMIT 1)
WHERE state = 'chosen';
