package requests

// Statements of settings.go. The text is the exact SQL the server runs.
const (
	readSettingsSQL  = "SELECT key, value FROM settings WHERE grp = 'this_app' AND key IN (?"
	saveSettingsSQL  = "SELECT count(*) FROM pick_rules WHERE id = ?"
	saveSettingsSQL2 = `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`
	setPersonLimitSQL  = "SELECT count(*) FROM users WHERE id = ?"
	setPersonLimitSQL2 = `INSERT INTO user_ask_settings (user_id, per_week, movies_per_week, shows_per_week, music_per_week) VALUES (?, NULL, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET per_week = NULL, movies_per_week = excluded.movies_per_week,
				shows_per_week = excluded.shows_per_week, music_per_week = excluded.music_per_week`
	readTargetsSQL = "SELECT email, discord_id, new_episodes = 1 FROM user_ask_settings WHERE user_id = ?"
	saveTargetsSQL = `INSERT INTO user_ask_settings (user_id, email, discord_id) VALUES (?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET email = excluded.email, discord_id = excluded.discord_id`
	saveTargetsSQL2 = "UPDATE user_ask_settings SET new_episodes = ? WHERE user_id = ?"
)
