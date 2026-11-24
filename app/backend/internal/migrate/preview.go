package migrate

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// preview reads the old app like /migrate/read but keeps nothing: no
// snapshot is saved and the key is not kept. It answers how much would
// come over, for the setup screen.
func (h *Handlers) preview(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	src, _, ok := h.source(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	snap, err := Read(ctx, src)
	if err != nil {
		fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"counts": previewCounts(snap)})
}

// previewCounts is how many of each thing a read found, zeros included.
func previewCounts(s *Snapshot) map[string]int {
	c := map[string]int{
		"titles": len(s.Titles), "folders": len(s.Folders),
		"quality_profiles": len(s.QualityProfiles), "custom_formats": len(s.CustomFormats),
		"release_profiles": len(s.ReleaseProfiles), "delay_profiles": len(s.DelayProfiles),
		"indexers": len(s.Indexers), "download_clients": len(s.DownloadClients),
		"import_lists": len(s.ImportLists), "exclusions": len(s.Exclusions),
		"notifications": len(s.Notifications), "app_links": len(s.AppLinks),
		"users": len(s.Users), "requests": len(s.Requests),
	}
	if s.Subtitles != nil {
		c["subtitle_languages"] = len(s.Subtitles.Languages)
		c["subtitle_profiles"] = len(s.Subtitles.Profiles)
		c["subtitle_sites"] = len(s.Subtitles.Providers)
	}
	if s.Downloader != nil {
		c["categories"] = len(s.Downloader.Categories)
		c["usenet_servers"] = len(s.Downloader.Servers)
	}
	return c
}
