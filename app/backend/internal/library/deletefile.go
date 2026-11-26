package library

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// Refusals of a single file delete.
var (
	ErrNotMovieFile  = errors.New("only a movie's file can be deleted on its own; for a show or an artist choose episodes or tracks")
	ErrNoSuchFile    = errors.New("the file chosen is not one of this movie's")
	ErrProtectedFile = errors.New("this copy is protected; remove its protection first")
)

// ErrFileKept is returned when the chosen file may not be deleted; its
// message is the reason the plan gives.
type ErrFileKept struct{ Why string }

func (e ErrFileKept) Error() string { return "the file can't be deleted: " + e.Why }

// planRemoveFile works out deleting one file of a movie. Nothing changes.
func planRemoveFile(ctx context.Context, db Reader, titleID, fileID int64, c RemoveChoice, now time.Time) (RemovePlan, mediafiles.Title, mediafiles.File, error) {
	var t mediafiles.Title
	var f mediafiles.File
	if err := database.Get(ctx, db, "titles", titleID, &t); err != nil {
		return RemovePlan{}, t, f, err
	}
	if t.State == "removed" {
		return RemovePlan{}, t, f, ErrAlreadyRemoved
	}
	if t.Kind != "movie" {
		return RemovePlan{}, t, f, ErrNotMovieFile
	}
	f, ok, err := mediafiles.TitleFile(ctx, db, t.ID, fileID)
	if err != nil {
		return RemovePlan{}, t, f, err
	}
	if !ok {
		return RemovePlan{}, t, f, ErrNoSuchFile
	}
	// A protected copy is never deleted from its row, acknowledged or not:
	// the owner removes its protection first.
	if f.Protected {
		return RemovePlan{}, t, f, ErrProtectedFile
	}
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil {
		return RemovePlan{}, t, f, err
	}
	p := RemovePlan{TitleID: t.ID, Name: t.Name, Scope: "file", Choice: c, Held: []RemoveFile{}, Kept: []RemoveFile{}, HoldingDays: rules.HoldingDays}
	if err := planFile(ctx, db, &p, t, f, c, rules.HoldingDays, now); err != nil {
		return p, t, f, err
	}
	if p.Skipped {
		for _, h := range p.Held {
			h.To = ""
			h.Why = ErrProtectedFile.Error()
			p.Kept = append(p.Kept, h)
		}
		p.Held = []RemoveFile{}
	}
	return p, t, f, nil
}

// RemoveFileOf deletes one file of a movie (apply false: only the plan). An
// imported or adopted file outside a look-only Library folder goes to the
// holding folder with the subtitles beside it, where Recently deleted can
// restore it. The movie stays: the Slots the file filled want a copy again,
// and Keep up to date goes off for it when the owner chose that for files
// deleted from disk.
func (s *Service) RemoveFileOf(ctx context.Context, titleID, fileID int64, c RemoveChoice, apply bool, by domain.Actor) (RemovePlan, error) {
	c = handsOffChoice(ctx, s.Store.Read(), c)
	unlock := s.Store.LockFiles()
	defer unlock()
	p, t, f, err := planRemoveFile(ctx, s.Store.Read(), titleID, fileID, c, s.now())
	if err != nil || !apply {
		return p, err
	}
	if p.Skipped {
		return p, ErrProtectedFile
	}
	if len(p.Held) == 0 {
		why := "it is kept"
		if len(p.Kept) > 0 {
			why = p.Kept[0].Why
		}
		return p, ErrFileKept{Why: why}
	}
	if err := s.holdFiles(ctx, &p, t.ID, by); err != nil {
		return p, err
	}
	// As when the file is deleted from disk: the owner's switch decides
	// whether the movie keeps looking for a copy.
	var lf mediafiles.LibraryFolder
	if database.Get(ctx, s.Store.Read(), "library_folders", p.Held[0].LibraryFolderID, &lf) == nil {
		if err := s.unkeepDeleted(ctx, lf, f, by); err != nil {
			return p, err
		}
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		return mediafiles.EmptySlotsOfFileTx(ctx, tx, f.ID, "its file was deleted", s.now(), by)
	})
	if err != nil {
		return p, err
	}
	s.removeEmptyFolders(ctx, t.ID, p.Held, false)
	p.Applied = true
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventMovieFileRemoved, TitleID: &t.ID, Summary: "Deleted " + filepath.Base(f.RelativePath) + " of " + t.Name,
		Data: map[string]any{"file_id": f.ID, "path": f.RelativePath, "delete_files": true, "held": len(p.Held)}, GroupKey: GroupKey(t.ID)})
	s.Publish("title", map[string]any{"id": t.ID})
	return p, nil
}
