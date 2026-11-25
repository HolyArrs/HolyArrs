package shows

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// SkipEpisodes switches single episodes of one season off. The season and
// its other episodes keep their choice; a number the season lacks is fine.
func (s *Service) SkipEpisodes(ctx context.Context, titleID int64, number int, episodes []int, by domain.Actor) error {
	if len(episodes) == 0 {
		return nil
	}
	var t mediafiles.Title
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.Kind != "show" {
			return library.ErrNotShow
		}
		ok, err := mediafiles.SkipEpisodesTx(ctx, tx, titleID, number, episodes, s.Clock(), by)
		if err == nil && !ok {
			err = library.ErrNoSuchSeason
		}
		return err
	})
	if err != nil {
		return err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventSeasonChanged, TitleID: &titleID,
		Summary: fmt.Sprintf("%s season %d: Keep up to date off for %d episodes", t.Name, number, len(episodes)), Data: map[string]any{"season": number, "episodes_off": episodes},
		GroupKey: library.GroupKey(titleID)})
	return nil
}
