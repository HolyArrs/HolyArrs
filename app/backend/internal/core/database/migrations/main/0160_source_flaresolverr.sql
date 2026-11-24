-- A Source may name a FlareSolverr to open the site's bot check (such as
-- Cloudflare's) when it shows one. Empty means none: the Source stays
-- Blocked as before.
ALTER TABLE sources ADD COLUMN flaresolverr_url TEXT NOT NULL DEFAULT '';
