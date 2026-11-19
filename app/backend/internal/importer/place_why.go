package importer

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Why a placing stopped, in words that say what to do. They are the reasons a
// movie or an episode waits for a person.
const (
	targetTakenWhy = "a file with the same name that HolyArrs did not place is already in the folder; move it out of the folder, then import again"
	noWriteWhy     = "HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again"
	noReadWhy      = "HolyArrs may not read the downloaded file; give the user HolyArrs runs as (PUID and PGID) read access to the file and the folder it is in, then import again"
	notThereWhy    = "HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again"
)

// What a plan step says when the file could not be read to check it, or was not there.
const (
	noReadCheckWhy = "HolyArrs may not read the file (permissions)"
	notInFolderWhy = "the file is not in the download folder any more"
	unreadCheckWhy = "could not read the file to check it"
)

// fileTakenWhy is the reason for a file that is gone while its download folder is still there.
const fileTakenWhy = "the file could not be placed: " + notInFolderWhy

// SlowReadWhy is the reason of a card whose Import again read the download
// folder for longer than its time limit.
const SlowReadWhy = "reading the download folder took too long; try again"

// CanImportAgain reports whether reason is one the person fixes outside
// HolyArrs (a path pair, a mount, the save folder, a permission), or an
// import that gave up on a passing fault, after which the same download can
// be imported again. So can one that matched no file: by hand finds nothing
// in a download that holds only archives.
func CanImportAgain(reason string) bool {
	return reason == noMovieWhy || reason == noTrackWhy || reason == noEpisodeWhy || reason == notThereWhy || reason == noReadWhy || reason == noWriteWhy || reason == gaveUpWhy || reason == SlowReadWhy || strings.Contains(reason, "HOLYARRS_SAVE_PATH")
}

// goneWhy is the reason for a file that is not there: the path pair and the
// mount to check when the download folder root is missing too, otherwise the
// file having gone from a folder that is there.
func goneWhy(root string) string {
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return notThereWhy
	}
	return fileTakenWhy
}

// unreadOr is why, or, when why refuses a file that could not be inspected
// and cannot be opened either, the reason it cannot be opened.
func unreadOr(why string, facts map[string]any, file string) string {
	if why == "" || facts["inspection_status"] != "error" {
		return why
	}
	f, err := os.Open(file)
	if err != nil {
		return unreadable(err)
	}
	_ = f.Close()
	return why
}

// placeWhy is the reason a failed placing gives: a plain line when the cause
// is a refused read of the downloaded file src or a refused write to the
// Library, otherwise the system's own words. The system's words of a plain
// line go to the log.
func placeWhy(err error, raw, src string) string {
	if !errors.Is(err, fs.ErrPermission) {
		return raw
	}
	log.Printf("importer: placing %s: %s", src, raw)
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Path == src {
		return noReadWhy
	}
	return noWriteWhy
}

// couldNotPlace is the reason for a file that matched but was not placed. The
// plain lines stand alone; the system's words follow a lead.
func couldNotPlace(why string) string {
	if why == targetTakenWhy || why == noWriteWhy || why == noReadWhy {
		return why
	}
	return "the file could not be placed: " + why
}

// unseenWhy is the reason nothing was matched because the download's folder
// root was never seen: the path pair and the mount to check when it is not
// there, the read access to give when it gave no file (noFiles) and may not be
// listed. It is "" when a file of the plan is there, so a match was tried.
func (im *Importer) unseenWhy(dr domain.Downloader, files []domain.SeenFile, root string, rootOK bool, plan []mediafiles.PlanStep, noFiles bool) string {
	if !rootOK || root == "" || im.unreadReason(dr, files) != "" {
		return ""
	}
	for _, s := range plan {
		if _, err := os.Lstat(s.SourceFile); err == nil && filepath.IsAbs(s.SourceFile) {
			return ""
		}
	}
	if goneWhy(root) == notThereWhy {
		return notThereWhy
	}
	if _, err := os.ReadDir(root); noFiles && errors.Is(err, fs.ErrPermission) {
		return noReadWhy
	}
	return ""
}

// nothingMatchedIn is an episode or album import's reason when no file
// matched: that the download's folder was never seen, when that is why.
func (im *Importer) nothingMatchedIn(dr domain.Downloader, files []domain.SeenFile, music bool, root string, rootOK bool, plan []mediafiles.PlanStep, noFiles bool) string {
	if why := im.unseenWhy(dr, files, root, rootOK, plan, noFiles); why != "" {
		return why
	}
	return im.nothingMatched(dr, files, music, plan)
}

// episodeNotPlacedWhy is the reason of an episode or track file that was
// meant to be placed and was not: for a file that is not there any more, the
// path pair and the mount to check when the download folder root is gone too,
// otherwise the file having gone.
func episodeNotPlacedWhy(root string, s mediafiles.PlanStep, r mediafiles.PlaceResult) string {
	if _, err := os.Lstat(s.SourceFile); errors.Is(err, fs.ErrNotExist) {
		return goneWhy(root)
	}
	return couldNotPlace(r.Error)
}

// episodeRefusedWhy is the reason of an episode or track file the plan
// refused for why: the path pair and the mount to check when the file was not
// found and the download folder root is not there either. A file that was
// gone or could not be read says that alone, without how it matched, so the
// line is one the app can show in the picked language.
func episodeRefusedWhy(root, label string, why []string) string {
	lead := "the file for " + label + " could not be placed: "
	switch {
	case slices.Contains(why, noReadCheckWhy):
		return noReadWhy
	case slices.Contains(why, notInFolderWhy) && goneWhy(root) == notThereWhy:
		return notThereWhy
	case slices.Contains(why, notInFolderWhy):
		return lead + notInFolderWhy
	case slices.Contains(why, unreadCheckWhy):
		return lead + unreadCheckWhy
	}
	return lead + strings.Join(why, "; ")
}

// notPlacedWhy is the reason of the first file a movie import meant to place
// and did not, or of a file it could not read to check or did not find, or ""
// when there is none. root is the download's folder as HolyArrs sees it. An
// import whose files another app took is judged before this and keeps its
// own reason.
func notPlacedWhy(root string, plan []mediafiles.PlanStep, res []mediafiles.PlaceResult) string {
	for i, r := range res {
		if i < len(plan) && plan[i].Method == "hardlink" && r.Outcome != "placed" && r.Error != "" {
			if _, err := os.Lstat(plan[i].SourceFile); errors.Is(err, fs.ErrNotExist) {
				return goneWhy(root)
			}
			return couldNotPlace(r.Error)
		}
	}
	for _, s := range plan {
		switch {
		case slices.Contains(s.Why, noReadCheckWhy):
			return noReadWhy
		case slices.Contains(s.Why, notInFolderWhy):
			return goneWhy(root)
		case slices.Contains(s.Why, unreadCheckWhy):
			return couldNotPlace(unreadCheckWhy)
		}
	}
	return ""
}
