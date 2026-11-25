-- Requests imported from Seerr, Overseerr or Jellyseerr that were already
-- approved there came over as waiting when their title was not in the
-- Library yet (marked by the note "approved in <app>: approve it again here
-- to add it"). They are approved: they move to Processing, keep the time
-- they were asked as their decision time, and lose that note.
UPDATE asks SET state = 'approved',
	decided_at = COALESCE(decided_at, asked_at),
	note = trim(substr(note, 1, instr(note, 'approved in ') - 1) || substr(note, instr(note, 'approve it again here to add it') + 31), '; ')
WHERE state = 'asked'
	AND instr(note, 'approved in ') > 0
	AND instr(note, 'approve it again here to add it') > instr(note, 'approved in ');
