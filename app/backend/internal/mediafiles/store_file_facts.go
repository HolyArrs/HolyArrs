package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// SetFileFactsTx stores what reading File f again gave: its media info,
// languages, size, quality, release name and group, stamped now by by.
func SetFileFactsTx(ctx context.Context, tx *sql.Tx, f File, now time.Time, by database.Actor) error {
	var cols [3][]byte
	for i, col := range []any{f.MediaInfo, f.Languages, f.Quality} {
		var err error
		if cols[i], err = json.Marshal(col); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE files SET media_info = json(?), languages = json(?), quality = json(?), size = ?, release_name = ?,
		release_group = ?, updated_at = ?, updated_by = ? WHERE id = ?`, string(cols[0]), string(cols[1]), string(cols[2]),
		f.Size, f.ReleaseName, f.Group, database.FormatTime(now), actorJSON(by), f.ID)
	return err
}
