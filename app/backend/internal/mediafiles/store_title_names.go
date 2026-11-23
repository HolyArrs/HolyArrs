package mediafiles

import (
	"context"
	"encoding/json"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TitleOtherNames is a title's other names: its aliases and its original name,
// used to match a release named after the full title to the one in the Library.
func TitleOtherNames(ctx context.Context, db database.Querier, id int64) ([]string, error) {
	var raw string
	var orig *string
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(aliases, '[]'), original_name FROM titles WHERE id = ?", id).Scan(&raw, &orig); err != nil {
		return nil, err
	}
	var names []string
	_ = json.Unmarshal([]byte(raw), &names)
	if orig != nil && *orig != "" {
		names = append(names, *orig)
	}
	return names, nil
}
