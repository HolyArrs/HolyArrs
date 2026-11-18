-- Which lines of "What people can ask for" one person may pick, per kind, as
-- JSON {"movie": [...], "show": [...]}. NULL (every row so far) or a kind
-- left out means every line that is shown, as before.
ALTER TABLE user_ask_settings ADD COLUMN qualities TEXT;
