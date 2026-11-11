package importer

import (
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
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

// nothingPlaced is a movie import's reason when nothing could be placed.
func (im *Importer) nothingPlaced(dr domain.Downloader, files []domain.SeenFile) string {
	if why := im.unreadReason(dr, files); why != "" {
		return why
	}
	return "nothing could be placed; look at the import plan"
}

// nothingMatched is an episode or album import's reason when no file
// matched an episode or a track.
func (im *Importer) nothingMatched(dr domain.Downloader, files []domain.SeenFile, music bool) string {
	if why := im.unreadReason(dr, files); why != "" {
		return why
	}
	if music {
		return "nothing in the download could be matched to a track; look at the import plan"
	}
	return "nothing in the download could be matched to an episode; look at the import plan"
}
