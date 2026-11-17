package library

import (
	"context"
	"database/sql"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// SavePushedTrait stores the trait a format sent through the arr API
// became: updated in place when the same format (sourceID, "<side>:<id>")
// comes again, else made. side words a taken name ("(shows)", "(movies)").
// It answers the trait's id.
func SavePushedTrait(ctx context.Context, st *database.Store, side, sourceID, name string, specs []mediafiles.OwnSpec, by domain.Actor) (int64, error) {
	var id int64
	err := st.Write(ctx, func(tx *sql.Tx) error {
		rows, err := database.Select[mediafiles.OwnTrait](ctx, tx, "own_traits", "source = ? AND source_id = ?", mediafiles.SourceArrAPI, sourceID)
		if err != nil {
			return err
		}
		var t mediafiles.OwnTrait
		if len(rows) > 0 {
			t = rows[0]
		}
		if specs == nil {
			specs = []mediafiles.OwnSpec{}
		}
		t.Name, err = freeTraitName(tx, name, side, t.ID)
		if err != nil {
			return err
		}
		t.Match, t.Terms, t.Specs = "name", []string{}, specs
		if t.ID != 0 {
			id = t.ID
			return database.Update(tx, "own_traits", &t, by)
		}
		t.Source, t.SourceID = mediafiles.SourceArrAPI, sourceID
		id, err = database.Insert(tx, "own_traits", &t, by)
		return err
	})
	if err != nil {
		return 0, err
	}
	reload(ctx, st)
	return id, nil
}

// PushedTraits maps each trait sent through the arr API to its source id.
func PushedTraits(ctx context.Context, st *database.Store) (map[int64]string, error) {
	rows, err := database.Select[mediafiles.OwnTrait](ctx, st.Read(), "own_traits", "source = ?", mediafiles.SourceArrAPI)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, t := range rows {
		out[t.ID] = t.SourceID
	}
	return out, nil
}
