package importer

import (
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// packReady keeps the files of a season pack fetched a few episodes at a time
// that may be placed: switched on, not skipped in the client and finished.
// waiting holds the Slots whose file is switched on but not finished yet. A
// normal download keeps every file.
func packReady(d domain.Download) ([]domain.SeenFile, map[int64]bool) {
	if d.PackFiles == nil {
		return d.FilesSeen, nil
	}
	on := map[string]bool{}
	for _, f := range d.PackFiles {
		on[f.Name] = f.On
	}
	var files []domain.SeenFile
	done := map[string]bool{}
	for _, f := range d.FilesSeen {
		if isOn, listed := on[f.Name]; listed && !isOn || f.Priority == 0 || f.Progress < 1 {
			continue
		}
		files = append(files, f)
		done[f.Name] = true
	}
	waiting := map[int64]bool{}
	for _, f := range d.PackFiles {
		if f.On && !done[f.Name] {
			for _, s := range f.Slots {
				waiting[s] = true
			}
		}
	}
	return files, waiting
}
