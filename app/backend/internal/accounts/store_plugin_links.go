package accounts

import "context"

// PluginLink is one plugin a person linked to their account.
type PluginLink struct {
	Plugin    string `json:"plugin"`
	External  string `json:"external"`
	CreatedAt string `json:"created_at"`
}

func addPluginLink(ctx context.Context, q conn, plugin string, userID int64, external, now string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO plugin_links (plugin_id, user_id, external, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (plugin_id, user_id) DO UPDATE SET external = excluded.external, created_at = excluded.created_at`, plugin, userID, external, now)
	return err
}

func removePluginLink(ctx context.Context, q conn, plugin string, userID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM plugin_links WHERE plugin_id = ? AND user_id = ?`, plugin, userID)
	return err
}

func pluginLinked(ctx context.Context, q conn, plugin string, userID int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM plugin_links l JOIN users u ON u.id = l.user_id
		WHERE l.plugin_id = ? AND l.user_id = ? AND u.state = 'active' AND u.removed_at = ''`, plugin, userID).Scan(&n)
	return n > 0, err
}

func pluginLinksOf(ctx context.Context, q conn, userID int64) ([]PluginLink, error) {
	rows, err := q.QueryContext(ctx, `SELECT plugin_id, external, created_at FROM plugin_links WHERE user_id = ? ORDER BY plugin_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PluginLink{}
	for rows.Next() {
		var l PluginLink
		if err := rows.Scan(&l.Plugin, &l.External, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
