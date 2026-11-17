-- A Subtitle site may let in files the site marks as machine-made
-- (machine_made, off by default), and a saved subtitle records what made it
-- when a person did not (made): HolyArrs's own translation, whole or partial,
-- or a site's AI-translated or machine-translated file. NULL made: a person's
-- subtitle. Add-only.
ALTER TABLE subtitle_sites ADD COLUMN machine_made INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subtitles ADD COLUMN made TEXT CHECK (made IN ('translated', 'translated_partial', 'ai_translated', 'machine_translated'));
