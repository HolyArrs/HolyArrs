package library

import (
	"context"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// folderErrs are the refusals of one kind of Library folder.
var folderErrs = map[string][2]error{
	"movie": {ErrNoFolder, ErrNotMovieFolder},
	"show":  {ErrNoShowFolder, ErrNotShowFolder},
	"music": {ErrNoMusicFolder, ErrNotMusicFolder},
}

// TitleFolder picks the Library folder a new title of kind (movie, show or
// music) goes into: id when given, else the first real folder of the kind.
// In hands-off mode a title with no folder of its kind goes under the
// stand-in folder, and hands is true: the folder is never checked on disk,
// since nothing is written there.
func TitleFolder(ctx context.Context, s *database.Store, id int64, kind string, by domain.Actor) (lf mediafiles.LibraryFolder, hands bool, err error) {
	db := s.Read()
	hands = parts.HandsOff(ctx, db)
	errs := folderErrs[kind]
	if id == 0 {
		lf, found, err := mediafiles.FirstRealFolder(ctx, db, kind)
		if err != nil {
			return lf, hands, err
		}
		if found {
			return lf, hands, nil
		}
		if !hands {
			return lf, false, errs[0]
		}
		lf, err = mediafiles.StandInFolder(ctx, s, kind, by)
		return lf, true, err
	}
	if err := database.Get(ctx, db, "library_folders", id, &lf); errors.Is(err, database.ErrNotFound) {
		return lf, hands, errs[0]
	} else if err != nil {
		return lf, hands, err
	}
	if lf.Kind != kind {
		return lf, hands, errs[1]
	}
	return lf, hands || lf.StandIn, nil
}

// handsOffChoice keeps every file in hands-off mode: removing a title then
// only takes it out of HolyArrs, whatever was asked.
func handsOffChoice(ctx context.Context, db database.Querier, c RemoveChoice) RemoveChoice {
	if c.DeleteFiles && parts.HandsOff(ctx, db) {
		c.DeleteFiles = false
	}
	return c
}
