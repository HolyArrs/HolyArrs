-- Kid accounts: the highest age rating a member may see and ask for, and
-- the country whose ratings apply. An empty max_rating means no limit; only
-- US ratings are kept for now. Invites carry the limit to the new person.
ALTER TABLE users ADD COLUMN max_rating TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN rating_country TEXT NOT NULL DEFAULT 'US';
ALTER TABLE invites ADD COLUMN max_rating TEXT NOT NULL DEFAULT '';
ALTER TABLE invites ADD COLUMN rating_country TEXT NOT NULL DEFAULT 'US';
