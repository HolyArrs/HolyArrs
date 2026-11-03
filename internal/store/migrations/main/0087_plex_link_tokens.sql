-- #102: a person's Plex sign-in token, kept sealed in the vault, so their
-- Plex watchlist can be read on a schedule. It belongs to the link: whatever
-- removes the link or keeps a newer token removes the old vault row in the
-- same write (internal/auth/plex_tokens.go).
ALTER TABLE user_links ADD COLUMN token_secret_id INTEGER;
