-- chosen = 1 once a person set their own Plex watchlist switches. Rows the
-- schedule writes for others keep 0, so they still follow the owner's
-- "everyone" switch. Rows from before were all set by their person.
ALTER TABLE plex_watchlist_people ADD COLUMN chosen INTEGER NOT NULL DEFAULT 0;
UPDATE plex_watchlist_people SET chosen = 1;
