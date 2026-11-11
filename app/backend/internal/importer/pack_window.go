package importer

import (
	"slices"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// packReady keeps the files of a season pack fetched a few episodes at a time
// that may be placed: switched on, not skipped in the client and finished.
// waiting holds the Slots whose file is switched on but not finished yet. A
// pack file is found by name, else by its place in the torrent's file list (a
// file renamed in the client). A file skipped in the client or not listed at
// all never comes, so nothing waits for it. A normal download keeps every
// file.
func packReady(d domain.Download) ([]domain.SeenFile, map[int64]bool) {
	if d.PackFiles == nil {
		return d.FilesSeen, nil
	}
	off := map[int]bool{}
	waiting := map[int64]bool{}
	for _, f := range d.PackFiles {
		i := slices.IndexFunc(d.FilesSeen, func(s domain.SeenFile) bool { return s.Name == f.Name })
		if i < 0 && f.I >= 0 && f.I < len(d.FilesSeen) {
			i = f.I
		}
		switch {
		case i < 0:
		case !f.On:
			off[i] = true
		case d.FilesSeen[i].Priority != 0 && d.FilesSeen[i].Progress < 1:
			for _, s := range f.Slots {
				waiting[s] = true
			}
		}
	}
	var files []domain.SeenFile
	for i, f := range d.FilesSeen {
		if off[i] || f.Priority == 0 || f.Progress < 1 {
			continue
		}
		files = append(files, f)
	}
	return files, waiting
}
