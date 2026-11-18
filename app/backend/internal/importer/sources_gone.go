package importer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// sourcesGone reports whether another app took a finished download's files:
// every video or audio file the plan would read is gone while the folder above
// the download is still there. Only a Usenet job (ownFolder) counts: another
// app moves a job's files out and the Downloader keeps the job in its
// history. A torrent the Downloader still lists with its files missing was not
// taken (other apps link or copy from a seeding torrent), so like Sonarr and
// Lidarr it waits for a decision instead.
func sourcesGone(root string, plan []mediafiles.PlanStep, ownFolder bool) bool {
	if !ownFolder || !filepath.IsAbs(root) {
		return false
	}
	if _, err := os.Stat(root); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		if _, err := os.Stat(filepath.Dir(root)); err != nil {
			return false
		}
	}
	// Only video or audio files count, refused ones included, unless the plan
	// has none (archives only): then its archives and side files count.
	anyMedia := slices.ContainsFunc(plan, func(st mediafiles.PlanStep) bool { return IsMedia(filepath.Ext(st.SourceFile)) })
	seen := false
	for _, st := range plan {
		if st.Method == "refuse" || !filepath.IsAbs(st.SourceFile) || (anyMedia && !IsMedia(filepath.Ext(st.SourceFile))) {
			continue
		}
		if _, err := os.Lstat(st.SourceFile); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		seen = true
	}
	if !seen {
		_, err := os.Lstat(root)
		return errors.Is(err, fs.ErrNotExist)
	}
	return true
}

// markGone records every step that was not placed as gone, with the reason.
func markGone(res []mediafiles.PlaceResult) {
	for i := range res {
		if res[i].Outcome != "placed" && res[i].Outcome != "skipped" {
			res[i].Outcome, res[i].Error = "gone", FilesGoneReason
		}
	}
}

// FilesGoneReason is why an import let its download go: every file it would
// read was already removed, as another app sharing the Downloader does after
// its own import.
const FilesGoneReason = "the files were removed by another app before HolyArrs could import them"
