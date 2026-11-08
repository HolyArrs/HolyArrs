package accounts

import "context"

// The field arguments below are column names the caller picks from a fixed
// set ("shown", "opens", "asks" or "opened"); they are never request text.

// AddRowCounts adds one to field for each row key on day.
func AddRowCounts(ctx context.Context, q conn, member int64, day, field string, keys []string) error {
	for _, k := range keys {
		if _, err := q.ExecContext(ctx, `INSERT INTO explore_counts (member_id, row_type, day, `+field+`) VALUES (?, ?, ?, 1)
				ON CONFLICT (member_id, row_type, day) DO UPDATE SET `+field+` = `+field+` + 1`, member, k, day); err != nil {
			return err
		}
	}
	return nil
}

// AddLayerCounts adds one to field for each title-layer key on day.
func AddLayerCounts(ctx context.Context, q conn, member int64, day, field string, keys []string) error {
	for _, k := range keys {
		if _, err := q.ExecContext(ctx, `INSERT INTO explore_title_counts (member_id, layer, day, `+field+`) VALUES (?, ?, ?, 1)
				ON CONFLICT (member_id, layer, day) DO UPDATE SET `+field+` = `+field+` + 1`, member, k, day); err != nil {
			return err
		}
	}
	return nil
}

// LayerTotal is one title layer's sums over a span of days.
type LayerTotal struct {
	Layer              string
	Shown, Opens, Asks int
}

// LayerTotals sums the title-layer counts of every person from day from on.
func LayerTotals(ctx context.Context, q conn, from string) ([]LayerTotal, error) {
	rows, err := q.QueryContext(ctx, `SELECT layer, SUM(shown), SUM(opens), SUM(asks) FROM explore_title_counts
		WHERE day >= ? GROUP BY layer ORDER BY layer`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LayerTotal
	for rows.Next() {
		var t LayerTotal
		if err := rows.Scan(&t.Layer, &t.Shown, &t.Opens, &t.Asks); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CategoryKeysOn counts the category keys a person has on day.
func CategoryKeysOn(ctx context.Context, q conn, member int64, day string) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM explore_category_counts WHERE member_id = ? AND day = ?`, member, day).Scan(&n)
	return n, err
}

// AddCategoryCount adds one to field for a category key on day, making the
// row when it is missing.
func AddCategoryCount(ctx context.Context, q conn, member int64, key, day, field string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO explore_category_counts (member_id, category_id, day, `+field+`) VALUES (?, ?, ?, 1)
				ON CONFLICT (member_id, category_id, day) DO UPDATE SET `+field+` = `+field+` + 1`, member, key, day)
	return err
}

// BumpCategoryCount adds one to field for a category key already counted
// on day; a missing row stays missing.
func BumpCategoryCount(ctx context.Context, q conn, member int64, key, day, field string) error {
	_, err := q.ExecContext(ctx, `UPDATE explore_category_counts SET `+field+` = `+field+` + 1 WHERE member_id = ? AND category_id = ? AND day = ?`, member, key, day)
	return err
}

// CategoryDay is one category key's counts on one day.
type CategoryDay struct {
	Key, Day      string
	Shown, Opened int
}

// CategoryCountsSince reads a person's category counts from day from on.
func CategoryCountsSince(ctx context.Context, q conn, member int64, from string) ([]CategoryDay, error) {
	rows, err := q.QueryContext(ctx, `SELECT category_id, day, shown, opened FROM explore_category_counts
		WHERE member_id = ? AND day >= ?`, member, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategoryDay
	for rows.Next() {
		var d CategoryDay
		if err := rows.Scan(&d.Key, &d.Day, &d.Shown, &d.Opened); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ResetCategoryCounts forgets every category count of a person.
func ResetCategoryCounts(ctx context.Context, q conn, member int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM explore_category_counts WHERE member_id = ?", member)
	return err
}

// PruneViewCounts drops view counts from days before day before.
func PruneViewCounts(ctx context.Context, q conn, before string) error {
	if _, err := q.ExecContext(ctx, "DELETE FROM explore_title_counts WHERE day < ?", before); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, "DELETE FROM explore_category_counts WHERE day < ?", before); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, "DELETE FROM explore_counts WHERE day < ?", before)
	return err
}
