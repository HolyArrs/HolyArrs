package importer

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// markMoves marks the steps whose file is moved into the Library instead of
// linked or copied: files of a finished Usenet download (nothing seeds them, and
// Clear finished deletes them anyway) and files HolyArrs unpacked itself. A
// Downloader HolyArrs may not change keeps its files, and so does anything that
// sits inside a Library folder.
func (im *Importer) markMoves(ctx context.Context, db database.Lister, plan []mediafiles.PlanStep, dr domain.Downloader, unpacked []Candidate) {
	if im.MayMove == nil || !im.MayMove(dr) {
		return
	}
	temp := map[string]bool{}
	for _, c := range unpacked {
		temp[c.Path] = true
	}
	var libraries []string
	if folders, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "1 = 1"); err == nil {
		for _, f := range folders {
			libraries = append(libraries, filepath.Clean(f.Path))
		}
	} else {
		return
	}
	for i := range plan {
		st := &plan[i]
		if st.Method != "hardlink" || (dr.Protocol != "usenet" && !temp[st.SourceFile]) {
			continue
		}
		src := filepath.Clean(st.SourceFile)
		inside := false
		for _, l := range libraries {
			if src == l || under(l, src) {
				inside = true
				break
			}
		}
		st.Move = !inside
	}
}

// moveProof records the size and time of each file about to be moved, so a
// retry after a crash can tell the moved file from someone else's. A file that
// already moved keeps the proof the attempt before it took.
func moveProof(plan []mediafiles.PlanStep, results []mediafiles.PlaceResult, prior []mediafiles.PlaceResult) {
	before := map[string]mediafiles.PlaceResult{}
	for _, r := range prior {
		before[r.SourceFile] = r
	}
	for i := range results {
		if i >= len(plan) || !plan[i].Move {
			continue
		}
		if fi, err := os.Lstat(plan[i].SourceFile); err == nil {
			results[i].MoveSize, results[i].MoveMod = fi.Size(), fi.ModTime().UnixNano()
		} else if b, ok := before[plan[i].SourceFile]; ok {
			results[i].MoveSize, results[i].MoveMod = b.MoveSize, b.MoveMod
		}
	}
}

// restoreMoves puts back files an earlier attempt moved into the Library and
// never recorded (it stopped before the end). Each one must still be the same
// file: the source gone, the target a plain file of the recorded size and time.
// The retry then starts from the download folder like any other attempt.
func restoreMoves(prior *mediafiles.Import) {
	if prior == nil {
		return
	}
	for i, r := range prior.Result {
		if i >= len(prior.Plan) || r.MoveSize <= 0 || r.Outcome == "placed" && r.PlacedBy != "move" {
			continue
		}
		st := prior.Plan[i]
		if !st.Move {
			continue
		}
		if _, err := os.Lstat(st.SourceFile); !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		fi, err := os.Lstat(st.TargetPath)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() != r.MoveSize || fi.ModTime().UnixNano() != r.MoveMod {
			continue
		}
		err = os.MkdirAll(filepath.Dir(st.SourceFile), 0o777)
		if err == nil {
			err = renameNoReplace(st.TargetPath, st.SourceFile)
		}
		if err != nil {
			log.Printf("import: could not put %s back to %s for the retry: %v", st.TargetPath, st.SourceFile, err)
		}
	}
}

// keepProof copies the recorded size and time of moved files onto fresh
// results, so every later save of a placing import keeps them.
func keepProof(results, recorded []mediafiles.PlaceResult) {
	for i := range results {
		if i < len(recorded) && results[i].SourceFile == recorded[i].SourceFile {
			results[i].MoveSize, results[i].MoveMod = recorded[i].MoveSize, recorded[i].MoveMod
		}
	}
}
