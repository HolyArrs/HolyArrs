package mediafiles

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// VersionIDsWithPickRules lists the Versions that use the Pick rules set.
func VersionIDsWithPickRules(ctx context.Context, q database.Lister, pickRulesID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT id FROM versions WHERE pick_rules_id = ? ORDER BY id", pickRulesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
