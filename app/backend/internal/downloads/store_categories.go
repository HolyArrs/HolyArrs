package downloads

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// fillKindCategory sets kind's category on Downloader id only while it is
// still empty, so an edit made meanwhile wins.
func fillKindCategory(ctx context.Context, s *database.Store, id int64, kind, c string) error {
	col := map[string]string{"movie": "category_movies", "show": "category_shows", "artist": "category_music"}[kind]
	if col == "" {
		return nil
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE downloaders SET "+col+" = ? WHERE id = ? AND "+col+" = ''", c, id)
		return err
	})
}

// importedTitleKind resolves the title type through a download's first slot.
func importedTitleKind(ctx context.Context, q database.Querier, slotID int64) (string, error) {
	var kind string
	err := q.QueryRowContext(ctx, `SELECT t.kind FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE s.id = ?`, slotID).Scan(&kind)
	return kind, err
}
