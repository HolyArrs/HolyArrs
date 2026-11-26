package importer

import (
	"path/filepath"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// unreadReason says why a download gave nothing to import when HolyArrs could
// not read its files at all: the Downloader gave no file list, and with no
// save folder (HOLYARRS_SAVE_PATH) and no path pair a finished job's folder is
// never read. The save folder is HolyArrs's own setting, so the line names both
// ways to set it up rather than a setting the Downloader lacks.
// It is "" when that is not the reason.
func (im *Importer) unreadReason(dr domain.Downloader, files []domain.SeenFile) string {
	if len(files) > 0 || strings.TrimSpace(im.SavePath) != "" {
		return ""
	}
	for _, pm := range dr.PathMap {
		if strings.TrimSpace(pm.Ours) != "" {
			return ""
		}
	}
	name := strings.TrimSpace(dr.Name)
	if name == "" {
		name = "the Downloader"
	}
	return "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and " + name + " has no path pair. " +
		"Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again"
}

// The reasons of an import that matched no file. The download may be imported
// again as it is: its files are still there, and what kept them from matching
// (a name HolyArrs could not read, archives it could not unpack) may be gone.
const (
	noMovieWhy   = "no file in the download matched this movie; match the files by hand"
	noTrackWhy   = "no file in the download matched a track; match the files by hand"
	noEpisodeWhy = "no file in the download matched an episode; match the files by hand"
)

// nothingPlaced is a movie import's reason when nothing could be placed.
func (im *Importer) nothingPlaced(dr domain.Downloader, files []domain.SeenFile, plan []mediafiles.PlanStep) string {
	if why := im.unreadReason(dr, files); why != "" {
		return why
	}
	if why := refusedMedia(plan); why != "" {
		return why
	}
	return noMovieWhy
}

// nothingMatched is an episode or album import's reason when no file
// matched an episode or a track.
func (im *Importer) nothingMatched(dr domain.Downloader, files []domain.SeenFile, music bool, plan []mediafiles.PlanStep) string {
	if why := im.unreadReason(dr, files); why != "" {
		return why
	}
	if why := refusedMedia(plan); why != "" {
		return why
	}
	if music {
		return noTrackWhy
	}
	return noEpisodeWhy
}

// whyOutsideFolder is the refusal for a reported file that would leave the
// download folder.
const whyOutsideFolder = "the Downloader reported a file outside the download folder"

// refusedMedia names the first video or audio file refused for being outside
// the download folder. Other refusals (no match, sample) keep the no-match
// lines. It is "" when none was.
func refusedMedia(plan []mediafiles.PlanStep) string {
	for _, st := range plan {
		if st.Method == "refuse" && IsMedia(filepath.Ext(st.SourceFile)) && len(st.Why) > 0 && st.Why[0] == whyOutsideFolder {
			return "HolyArrs refused " + filepath.Base(filepath.FromSlash(st.SourceFile)) + ": " + strings.Join(st.Why, "; ")
		}
	}
	return ""
}

// filesWait is how long after a download finished an empty folder read is
// taken as files still on their way rather than an empty download.
const filesWait = 10 * time.Minute

// waitForFiles turns a no-match reason into a wait when the folder read found
// no file at all and the download finished a short while ago: on a network
// share the job's folder can show up a moment after the Downloader is done
// (#861). The wait tries again on its own and reads the folder again; after
// filesWait an empty read is the plain no-match decision again.
func waitForFiles(d domain.Download, reason string, now time.Time) string {
	if len(d.FilesSeen) > 0 || d.FinishedAt == nil || now.Sub(*d.FinishedAt) >= filesWait {
		return reason
	}
	if reason == noMovieWhy || reason == noEpisodeWhy || reason == noTrackWhy {
		return NotPresentYetWhy
	}
	return reason
}
