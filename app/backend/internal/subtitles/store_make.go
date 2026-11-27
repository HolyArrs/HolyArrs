package subtitles

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// savedFor is the subtitle the app saved that a person's pick for w
// replaces, or nil. The saved file may carry a region, as a pt-BR file for a
// pt wish.
func (s *Service) savedFor(ctx context.Context, fileID int64, w Want) *Subtitle {
	old, _ := database.Select[Subtitle](ctx, s.Store.Read(), "subtitles",
		"file_id = ? AND place = 'beside' AND state = 'present' AND origin IN ('fetched', 'by_hand') AND forced = ?", fileID, w.Forced)
	for i := range old {
		if LangMatches(w.Language, old[i].Language) {
			return &old[i]
		}
	}
	return nil
}

// markByHand records the subtitle saved at rel as a person's pick, so timed
// better searches leave it alone.
func (s *Service) markByHand(ctx context.Context, fileID int64, rel string) {
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE subtitles SET origin = 'by_hand' WHERE file_id = ? AND relative_path = ? AND state = 'present'", fileID, rel)
		return err
	})
}

// siteByID is one subtitle site as stored.
func siteByID(ctx context.Context, db database.Querier, id int64) (SiteRow, error) {
	var r SiteRow
	err := database.Get(ctx, db, "subtitle_sites", id, &r)
	return r, err
}
