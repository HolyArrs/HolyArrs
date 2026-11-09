package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// reset removes what seed added, through the app's own delete paths: the
// Held and Turned down Releases leave their lists, the titles are deleted
// from the Library (as Delete does, so their History stays), the SABnzbd
// Downloader and the Library folders it added are removed when nothing
// uses them, and every file it wrote is deleted. Files it did not write
// (the lab's own media) are never touched: only files that start with the
// placeholder mark are deleted.
func (w *world) reset(ctx context.Context) error {
	if len(w.m.Titles) == 0 && len(w.m.Paths) == 0 && len(w.m.Releases) == 0 {
		w.say("nothing to reset: no demo seed is recorded")
		return nil
	}
	db := w.s.Read()
	for _, id := range w.m.Releases {
		var st string
		if db.QueryRowContext(ctx, "SELECT state FROM releases WHERE id = ?", id).Scan(&st) != nil {
			continue
		}
		var err error
		switch st {
		case "held":
			_, err = w.sr.RemoveHeld(ctx, id, w.by)
		case "turned_down":
			_, err = w.sr.RemoveTurnedDown(ctx, id, w.by)
		}
		if err != nil {
			w.say("   Release %d: %v", id, err)
		}
	}
	keys := make([]string, 0, len(w.m.Titles))
	for k := range w.m.Titles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	removed := 0
	for _, k := range keys {
		id := w.m.Titles[k]
		var st, name string
		if db.QueryRowContext(ctx, "SELECT state, name FROM titles WHERE id = ?", id).Scan(&st, &name) != nil || st == "removed" {
			continue
		}
		if _, err := w.lib.Remove(ctx, id, library.RemoveChoice{}, true, w.by); err != nil {
			w.say("   %s: %v", name, err)
			continue
		}
		removed++
	}
	// Jobs seed queued for its titles (refresh, folder scan) are cancelled.
	_ = w.s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range w.m.Titles {
			if _, err := tx.Exec(`UPDATE jobs SET state = 'cancelled', ended_at = ?, updated_at = ? WHERE state = 'queued'
				AND json_extract(subject, '$.type') = 'title' AND json_extract(subject, '$.id') = ?`,
				database.FormatTime(w.o.now()), database.FormatTime(w.o.now()), id); err != nil {
				return err
			}
		}
		return nil
	})
	for _, id := range w.m.Downloaders {
		if err := downloads.Remove(ctx, w.s, id, w.by); err != nil && !errors.Is(err, database.ErrNotFound) {
			w.say("   Downloader %d: %v", id, err)
		}
	}
	for _, id := range w.m.Folders {
		if err := library.DeleteFolder(ctx, w.s, id); err != nil && !errors.Is(err, database.ErrNotFound) {
			w.say("   Library folder %d kept: %v", id, err)
		}
	}
	files, dirs := w.removeFiles()
	w.say("   %d titles deleted from the Library, %d files and %d folders deleted", removed, files, dirs)
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM settings WHERE grp = 'this_app' AND key = ?", manifestKey)
		return err
	}); err != nil {
		return err
	}
	w.m = newManifest()
	return nil
}

// removeFiles deletes every file seed wrote (its own list, plus imported
// copies and held files anywhere under the media and downloads folders
// that start with the placeholder mark), then the folders it made or
// emptied.
func (w *world) removeFiles() (int, int) {
	nf, nd := 0, 0
	emptied := map[string]bool{}
	del := func(p string) {
		if isPlaceholder(p) && os.Remove(p) == nil {
			nf++
			emptied[filepath.Dir(p)] = true
		}
	}
	for _, p := range w.m.Paths {
		if !strings.HasSuffix(p, string(filepath.Separator)) {
			del(p)
		}
	}
	for _, root := range []string{w.o.MediaRoot, w.Downloads} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				del(p)
			}
			return nil
		})
	}
	var dirs []string
	for _, p := range w.m.Paths {
		if strings.HasSuffix(p, string(filepath.Separator)) {
			dirs = append(dirs, strings.TrimSuffix(p, string(filepath.Separator)))
		}
	}
	for d := range emptied {
		for x := d; x != w.o.MediaRoot && x != w.Downloads && x != filepath.Dir(x) && strings.HasPrefix(x, filepath.Dir(w.o.MediaRoot)); x = filepath.Dir(x) {
			dirs = append(dirs, x)
		}
	}
	// Deepest first; only empty folders go, and never the roots or their
	// movies, tv and music folders.
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) > strings.Count(dirs[j], string(filepath.Separator))
	})
	keep := map[string]bool{w.o.MediaRoot: true, w.Downloads: true}
	for _, k := range []string{"movies", "tv", "shows", "music"} {
		keep[filepath.Join(w.o.MediaRoot, k)] = true
	}
	for _, d := range dirs {
		if keep[d] {
			continue
		}
		if ents, err := os.ReadDir(d); err == nil && len(ents) == 0 && os.Remove(d) == nil {
			nd++
		}
	}
	return nf, nd
}

// isPlaceholder says whether a file starts with the placeholder mark.
func isPlaceholder(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, len(placeholderMark))
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}
	return bytes.Equal(buf, []byte(placeholderMark))
}
