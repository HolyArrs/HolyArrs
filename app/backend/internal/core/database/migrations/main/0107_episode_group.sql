-- The TMDB episode order a show follows (a TMDB episode group id). NULL
-- keeps TMDB's own numbering.
ALTER TABLE titles ADD COLUMN episode_group_id TEXT;
