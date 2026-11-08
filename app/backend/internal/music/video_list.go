package music

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ArtistVideoItem is one of an artist's official music videos in the
// artist page's Music videos row: its YouTube id, title and length.
type ArtistVideoItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// artistVideoList answers GET /api/v1/music/artists/{mbid}?part=videos: the
// artist's music videos as the server keeps them, in the kept order. Nothing
// is looked up on YouTube or relayed here; a video plays when it is opened.
// No videos (or no video source) answers an empty list.
func (h *Handlers) artistVideoList(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	out := []ArtistVideoItem{}
	if h.ArtistVideos != nil {
		for _, v := range h.videosOf(metadata.MemberLookup(r.Context()), mbid) {
			if metadata.ValidYouTubeID(v.ID) {
				out = append(out, ArtistVideoItem{ID: v.ID, Title: v.Title, Duration: v.Duration})
			}
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"videos": out})
}
