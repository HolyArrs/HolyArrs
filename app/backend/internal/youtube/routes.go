package youtube

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Handlers serves the YouTube routes.
type Handlers struct {
	Svc *Service
}

// Routes registers the YouTube routes. The caller registers them only while
// the youtube feature switch is on.
func (h *Handlers) Routes(m *webserver.Router) {
	m.Handle("GET /api/v1/youtube/status", webserver.PermYouTube, http.HandlerFunc(h.getStatus))
	m.Handle("GET /api/v1/settings/youtube", webserver.NeedUsers, http.HandlerFunc(h.getSettings))
	m.Handle("PUT /api/v1/settings/youtube", webserver.NeedOwner, http.HandlerFunc(h.putSettings))
	m.Handle("POST /api/v1/youtube/probe", webserver.PermYouTube, http.HandlerFunc(h.probe))
	m.Handle("GET /api/v1/youtube/videos", webserver.PermYouTube, http.HandlerFunc(h.listVideos))
	m.Handle("POST /api/v1/youtube/videos", webserver.PermYouTube, http.HandlerFunc(h.addVideo))
	m.Handle("GET /api/v1/youtube/videos/{id}", webserver.PermYouTube, http.HandlerFunc(h.getVideo))
	m.Handle("DELETE /api/v1/youtube/videos/{id}", webserver.PermYouTube, http.HandlerFunc(h.removeVideo))
	m.Handle("GET /api/v1/youtube/videos/{id}/file", webserver.PermYouTube, http.HandlerFunc(h.videoFile))
	m.Handle("GET /api/v1/youtube/videos/{id}/thumb", webserver.PermYouTube, http.HandlerFunc(h.videoThumb))
	m.Handle("PUT /api/v1/youtube/videos/{id}/position", webserver.PermYouTube, http.HandlerFunc(h.savePosition))
	m.Handle("POST /api/v1/youtube/videos/{id}/retry", webserver.PermYouTube, http.HandlerFunc(h.retryVideo))
	m.Handle("GET /api/v1/youtube/follows", webserver.PermYouTube, http.HandlerFunc(h.listFollows))
	m.Handle("POST /api/v1/youtube/follows", webserver.PermYouTube, http.HandlerFunc(h.addFollow))
	m.Handle("PUT /api/v1/youtube/follows/{id}", webserver.PermYouTube, http.HandlerFunc(h.updateFollow))
	m.Handle("DELETE /api/v1/youtube/follows/{id}", webserver.PermYouTube, http.HandlerFunc(h.unfollow))
	m.Handle("POST /api/v1/youtube/adopt", webserver.NeedOwner, http.HandlerFunc(h.adopt))
}

// caller is the signed-in person. A kid gets 404 on every YouTube route,
// whatever they are ticked for. Without sign-in (tests) it is the owner.
func (h *Handlers) caller(w http.ResponseWriter, r *http.Request) (webserver.Caller, bool) {
	c, ok := webserver.From(r.Context())
	if !ok {
		return webserver.Caller{UserID: 1, Role: webserver.RoleOwner}, true
	}
	if c.MaxRating != "" {
		http.NotFound(w, r)
		return webserver.Caller{}, false
	}
	return c, true
}

func (h *Handlers) getStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.caller(w, r); !ok {
		return
	}
	webserver.WriteJSON(w, http.StatusOK, h.Svc.Status(r.Context()))
}
