package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const watchFoldersKey = "watch_folders"

// WatchEvery is how often the watched folders are looked through.
const WatchEvery = 5 * time.Minute

// watchSettle is how long a file must sit unchanged before it is taken,
// so a file still being copied in is left alone.
const watchSettle = 2 * time.Minute

// ReadWatchFolders loads the folders HolyArrs imports from on its own.
func ReadWatchFolders(ctx context.Context, db database.Lister) ([]string, error) {
	out := []string{}
	rows, err := db.QueryContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", importFoldersGroup, watchFoldersKey)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	if rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return out, err
		}
		var list []string
		if json.Unmarshal([]byte(raw), &list) == nil {
			if c, err := CleanImportFolders(list); err == nil {
				out = c
			}
		}
	}
	return out, rows.Err()
}

// SaveWatchFolders validates and stores the watched folders.
func SaveWatchFolders(ctx context.Context, s *database.Store, in []string, by domain.Actor) ([]string, error) {
	list, err := CleanImportFolders(in)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if filepath.IsAbs(p) && UnsafeFolder(p, s.Dir()) {
			return nil, ErrImportFolder
		}
	}
	raw, _ := json.Marshal(list)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			importFoldersGroup, watchFoldersKey, string(raw), now, actorText(by), now, actorText(by))
		return err
	})
	return list, err
}

// watchSeen remembers files a pass left alone, by path, size and change
// time, so the same file is not tried (and logged) again every pass.
var watchSeen = struct {
	sync.Mutex
	m map[string]string
}{m: map[string]string{}}

// WatchPass imports the files in the watched folders that match a wanted
// Slot with no file yet, moving them into the Library. Anything unsure
// (no match, a weak match, a sample, a Slot that already has a file) stays
// where it is for Import files by hand.
func (im *Importer) WatchPass(ctx context.Context) error {
	folders, err := ReadWatchFolders(ctx, im.Store.Read())
	if err != nil || len(folders) == 0 {
		return err
	}
	// A watched folder may overlap a Downloader's folder: files there may
	// still be seeding, so they are copied, never moved.
	places, err := im.ByHandPlaces(ctx)
	if err != nil {
		return err
	}
	dl := ByHandPlaces{Downloads: append(append([]string{}, places.Downloaders...), places.Downloads...)}
	now := time.Now()
	for _, root := range folders {
		if _, err := os.Stat(root); err != nil {
			continue // not there (yet): nothing to do
		}
		scan, err := im.ScanByHand(ctx, root)
		if err != nil {
			log.Printf("watched folder %s: %v", root, err)
			continue
		}
		var items []ByHandItem
		for _, f := range scan.Files {
			fi, err := os.Stat(f.Path)
			if err != nil || now.Sub(fi.ModTime()) < watchSettle {
				continue
			}
			mark := watchMark(fi)
			watchSeen.Lock()
			skip := watchSeen.m[f.Path] == mark
			watchSeen.Unlock()
			if skip {
				continue
			}
			g := f.Guess
			if !watchTakes(f) {
				watchSeen.Lock()
				watchSeen.m[f.Path] = mark
				watchSeen.Unlock()
				continue
			}
			mode := "move"
			if dl.within(filepath.Clean(f.Path)) != "" {
				mode = "copy"
			}
			items = append(items, ByHandItem{Path: f.Path, SlotID: g.SlotID, Mode: mode})
		}
		if len(items) == 0 {
			continue
		}
		res, err := im.ImportByHand(ctx, items, domain.System)
		if err != nil {
			return err
		}
		for _, r := range res {
			if r.Outcome == "placed" {
				log.Printf("watched folder: imported %s", r.Path)
				continue
			}
			log.Printf("watched folder: left %s: %s", r.Path, r.Error)
			if fi, err := os.Stat(r.Path); err == nil {
				watchSeen.Lock()
				watchSeen.m[r.Path] = watchMark(fi)
				watchSeen.Unlock()
			}
		}
	}
	return nil
}

// watchTakes reports whether a scanned file is sure enough to import
// without a person looking.
func watchTakes(f ByHandFile) bool {
	g := f.Guess
	return !f.InLibrary && f.Note == "" && g.Reason == "" && g.SlotID > 0 && !g.HasFile && g.Confidence >= MinConfidence
}

func watchMark(fi os.FileInfo) string {
	return fmt.Sprintf("%d|%d", fi.ModTime().UnixNano(), fi.Size())
}
