package requests

// Statements of episodes.go. The text is the exact SQL the server runs.
const (
	noteEpisodesSQL = `INSERT INTO new_episode_notices (title_id, slot_id, arrived_at)
				SELECT i.title_id, s.id, ? FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
				WHERE s.id = ? AND i.kind = 'episode' AND t.kind = 'show'
				ON CONFLICT (slot_id) DO NOTHING`
	noteEpisodesSQL2 = `INSERT INTO new_album_notices (title_id, part_id, arrived_at)
				SELECT i.title_id, i.part_id, ? FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
				WHERE s.id = ? AND i.kind = 'track' AND t.kind = 'artist' AND i.part_id IS NOT NULL
				ON CONFLICT (part_id) DO NOTHING`
	sendEpisodeNoticesSQL = `SELECT n.title_id, t.name, count(*) FROM new_episode_notices n JOIN titles t ON t.id = n.title_id
		WHERE n.sent_at IS NULL GROUP BY n.title_id HAVING min(n.arrived_at) <= ? ORDER BY n.title_id`
	sendEpisodeNoticesSQL2 = "UPDATE new_episode_notices SET sent_at = ? WHERE title_id = ? AND sent_at IS NULL"
	sendEpisodeNoticesSQL3 = "INSERT INTO messages (created_at, user_id, kind, text, ask_id, title_id) VALUES (?, ?, 'new_episodes', ?, ?, ?)"
	showPeopleSQL          = `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), min(w.ask_id)
		FROM (SELECT p.user_id, a.id AS ask_id FROM asks a JOIN ask_people p ON p.ask_id = a.id
				WHERE a.kind = 'show' AND a.title_id = ? AND a.state IN ('approved', 'available')
			UNION ALL SELECT f.user_id, NULL FROM follows f WHERE f.title_id = ?) w
		JOIN users u ON u.id = w.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE u.state != 'disabled' AND coalesce(x.new_episodes, 1) = 1
		GROUP BY u.id ORDER BY u.id`
)
