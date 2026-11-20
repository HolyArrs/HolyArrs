package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// MetadataNotAnswering is why a folder could not be looked up or brought in
// while the metadata server is not answering.
const MetadataNotAnswering = "The server HolyArrs gets details from is not answering. Scan again in a few minutes to bring this folder in."

// NotLookedUp is the state of a scanned folder the metadata server did not
// answer for: not known to match or not, unlike unmatched.
const NotLookedUp = "not_looked_up"

// MetadataNotAnsweringScan is the scan's own line when lookups failed that way.
const MetadataNotAnsweringScan = "The server HolyArrs gets details from is not answering, so some folders could not be looked up. Scan again in a few minutes."

// MetadataDown says err is the metadata server not answering (down, slow,
// busy, or waiting before it is tried again).
func MetadataDown(err error) bool {
	return errors.Is(err, metadata.ErrServiceUnavailable) || errors.Is(err, metadata.ErrServiceBusy) || errors.Is(err, outbound.ErrSuppressed)
}

// adoptWhy says in plain words why a folder was not brought in.
func adoptWhy(ctx context.Context, db Reader, err error) string {
	var dup interface{ InLibrary() int64 }
	switch {
	case MetadataDown(err):
		return MetadataNotAnswering
	case errors.As(err, &dup):
		var t mediafiles.Title
		if database.Get(ctx, db, "titles", dup.InLibrary(), &t) == nil && t.Name != "" {
			return fmt.Sprintf("%s is already in the Library in another folder", t.Name)
		}
	}
	return err.Error()
}

// adoptFailed writes down why a folder was not brought in. A folder another
// import brought in at the same moment counts as brought in, with its title.
func (s *Service) adoptFailed(ctx context.Context, lf mediafiles.LibraryFolder, r *AdoptResult, err error) {
	var dup interface{ InLibrary() int64 }
	if errors.As(err, &dup) {
		fold := FoldsCase(lf.Path)
		if owners, e := mediafiles.AdoptedFileOwners(ctx, s.Store.Read(), lf.ID, fold, r.Folder); e == nil && owners[FolderKey(r.Folder, fold)].Assigned && owners[FolderKey(r.Folder, fold)].TitleID == dup.InLibrary() {
			r.TitleID = dup.InLibrary()
			r.File = r.Folder
			return
		}
		if id, e := FolderTaken(ctx, s.Store.Read(), lf, r.Folder); e == nil && id != 0 && id == dup.InLibrary() {
			r.TitleID = id
			return
		}
	}
	r.Error = adoptWhy(ctx, s.Store.Read(), err)
}
