package downloads

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// folderTeller is a Usenet engine that tells where it puts finished jobs: its
// main folder for them first, then the folders of the given categories, as
// the Downloader sees them.
type folderTeller interface {
	finishedFolders(ctx context.Context, cats []string) ([]string, error)
}

// learnTimeout bounds the folder question on an owner's add, edit or test.
const learnTimeout = 10 * time.Second

var errSaveRootSet = errors.New("the Downloader has path pairs now")

// LearnSaveRoot is for an owner's add, edit or test of a Usenet Downloader with
// no path pair. It asks the Downloader where it puts finished jobs and, when
// HolyArrs sees that folder at the same path, keeps it as a path pair with the
// same path on both sides: the Downloader's save root, shown and editable in
// Settings, inside which imports read finished jobs. A folder a Downloader
// reports later, at import time, is never trusted. It returns the Downloader
// as saved and the folders it kept; any trouble just keeps nothing.
func (m *Manager) LearnSaveRoot(ctx context.Context, d domain.Downloader, by domain.Actor) (domain.Downloader, []string) {
	if len(d.PathMap) > 0 || d.Protocol != "usenet" || d.Removed || m.Store == nil {
		return d, nil
	}
	e, err := m.EngineFor(ctx, d)
	if err != nil {
		return d, nil
	}
	t, ok := e.(folderTeller)
	if !ok {
		return d, nil
	}
	ctx, cancel := context.WithTimeout(ctx, learnTimeout)
	defer cancel()
	theirs, err := t.finishedFolders(ctx, d.Categories())
	if err != nil {
		return d, nil
	}
	var kept []string
	for _, f := range theirs {
		// One folder inside another adds nothing; the global save path
		// already covers its own.
		if f = m.seenHere(f); f != "" && !underAny(f, kept) && !underAny(f, []string{m.SavePath}) {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		return d, nil
	}
	pm := make([]domain.PathMap, 0, len(kept))
	for _, f := range kept {
		pm = append(pm, domain.PathMap{Theirs: f, Ours: f})
	}
	saved, err := keepSaveRoot(ctx, m.Store, d.ID, pm, by)
	if err != nil {
		return d, nil
	}
	return saved, kept
}

// seenHere returns f cleaned when HolyArrs sees that folder at the same path:
// a full path to a folder here, never a whole disk, and never HolyArrs's own
// config folder or a folder that holds it.
func (m *Manager) seenHere(f string) string {
	f = strings.TrimSpace(f)
	if !absPath(f) || strings.Contains(f, "${") {
		return ""
	}
	c := filepath.Clean(f)
	if filepath.Dir(c) == c {
		return ""
	}
	if cfg := strings.TrimSpace(m.ConfigDir); cfg != "" {
		if abs, err := filepath.Abs(cfg); err == nil {
			cfg = abs
		}
		if underAny(c, []string{cfg}) || underAny(cfg, []string{c}) {
			return ""
		}
	}
	if fi, err := os.Stat(c); err != nil || !fi.IsDir() {
		return ""
	}
	return c
}

// keepSaveRoot stores pm as Downloader id's path pairs, unless it has some by
// now or was removed.
func keepSaveRoot(ctx context.Context, s *database.Store, id int64, pm []domain.PathMap, by domain.Actor) (domain.Downloader, error) {
	var d domain.Downloader
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "downloaders", id, &d); err != nil {
			return err
		}
		if d.Removed || len(d.PathMap) > 0 {
			return errSaveRootSet
		}
		d.PathMap = pm
		return database.Update(tx, "downloaders", &d, by)
	})
	return d, err
}
