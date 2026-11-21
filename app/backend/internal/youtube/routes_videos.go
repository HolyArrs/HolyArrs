package youtube

import (
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// probeView is the confirm card for a pasted link. Count is 0 when yt-dlp
// does not say, as for channels.
type probeView struct {
	Kind            string      `json:"kind"`
	ID              string      `json:"id"`
	URL             string      `json:"url"`
	Title           string      `json:"title"`
	ChannelName     string      `json:"channel_name"`
	ChannelID       string      `json:"channel_id"`
	DurationSeconds int         `json:"duration_seconds"`
	Count           int         `json:"count"`
	LiveStatus      string      `json:"live_status"`
	Entries         []entryView `json:"entries"`
}

type entryView struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	DurationSeconds int        `json:"duration_seconds"`
	LiveStatus      string     `json:"live_status"`
	UploadedAt      *time.Time `json:"uploaded_at"`
	Short           bool       `json:"short"`
}

func viewProbe(p Probe) probeView {
	v := probeView{Kind: p.Link.Kind, ID: p.Link.ID, URL: p.Link.URL, Title: p.Title, ChannelName: p.ChannelName,
		ChannelID: p.ChannelID, DurationSeconds: p.DurationSeconds, Count: p.Count, LiveStatus: p.LiveStatus, Entries: []entryView{}}
	for _, e := range p.Entries {
		ev := entryView{ID: e.ID, Title: e.Title, DurationSeconds: e.DurationSeconds, LiveStatus: e.LiveStatus, Short: e.Short}
		if e.Timestamp > 0 {
			t := time.Unix(e.Timestamp, 0).UTC()
			ev.UploadedAt = &t
		}
		v.Entries = append(v.Entries, ev)
	}
	return v
}

// fail answers an error from the service. Another person's row answers 404
// exactly like a missing one, with nothing of the row in the body.
func (h *Handlers) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		webserver.Fail(w, http.StatusNotFound, "not found")
	case IsRefusal(err):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNotFailed):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNeedsTools):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrLookup):
		// yt-dlp's words about the caller's own link, for the caller only.
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	default:
		webserver.Internal(w, "youtube", err)
	}
}

// pathID reads {id}; anything but a positive number answers 404.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

func (h *Handlers) probe(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.caller(w, r); !ok {
		return
	}
	var in struct {
		URL string `json:"url"`
	}
	if !webserver.Decode(w, r, &in, `send {"url": "https://www.youtube.com/watch?v=..."}`) {
		return
	}
	p, err := h.Svc.Probe(r.Context(), in.URL)
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, viewProbe(p))
}

func (h *Handlers) listVideos(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var f VideoFilter
	for _, n := range []struct {
		key string
		to  *int
	}{{"limit", &f.Limit}, {"offset", &f.Offset}} {
		if s := q.Get(n.key); s != "" {
			v, err := strconv.Atoi(s)
			if err != nil || v < 0 {
				webserver.Fail(w, http.StatusBadRequest, "limit and offset must be numbers of 0 or more")
				return
			}
			*n.to = v
		}
	}
	if s := q.Get("follow"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil || v <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "follow must be a follow id")
			return
		}
		f.FollowID = v
	}
	f.ChannelID = q.Get("channel")
	f.Unwatched = q.Get("unwatched") == "1"
	list, err := VideosOf(r.Context(), h.Svc.Store.Read(), c.UserID, f)
	if err != nil {
		h.fail(w, err)
		return
	}
	if list == nil {
		list = []Video{}
	}
	if err := WithFollows(r.Context(), h.Svc.Store.Read(), c.UserID, list); err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, list)
}

func (h *Handlers) addVideo(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	var in struct {
		URL       string `json:"url"`
		MaxHeight int    `json:"quality_max_height"`
	}
	if !webserver.Decode(w, r, &in, `send {"url": "https://www.youtube.com/watch?v=...", "quality_max_height": 1080}`) {
		return
	}
	v, err := h.Svc.AddVideo(r.Context(), c.UserID, in.URL, in.MaxHeight)
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, v)
}

func (h *Handlers) getVideo(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := VideoOf(r.Context(), h.Svc.Store.Read(), c.UserID, id)
	if err == nil {
		err = h.Svc.fill(r.Context(), h.Svc.Store.Read(), &v)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *Handlers) removeVideo(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.Svc.RemoveVideo(r.Context(), c.UserID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) listFollows(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	list, err := FollowsOf(r.Context(), h.Svc.Store.Read(), c.UserID)
	if err != nil {
		h.fail(w, err)
		return
	}
	if list == nil {
		list = []Follow{}
	}
	webserver.WriteJSON(w, http.StatusOK, list)
}

func (h *Handlers) addFollow(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	var in AddFollow
	if !webserver.Decode(w, r, &in, `send {"url": "https://www.youtube.com/@name", "backfill": 20, "keep_last": null, "include_shorts": false, "quality_max_height": 1080, "mirror": false}`) {
		return
	}
	f, err := h.Svc.AddFollow(r.Context(), c.UserID, in)
	if errors.Is(err, ErrDuplicateFollow) {
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "existing_id": f.ID})
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusCreated, f)
}

func (h *Handlers) updateFollow(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		MaxHeight     *int  `json:"quality_max_height"`
		IncludeShorts *bool `json:"include_shorts"`
		Enabled       *bool `json:"enabled"`
		KeepLast      *int  `json:"keep_last"`
		Mirror        *bool `json:"mirror"`
	}
	keys, ok := webserver.DecodeKeys(w, r, &in, `send any of {"quality_max_height": 720, "include_shorts": true, "enabled": false, "keep_last": 10, "mirror": true}; "keep_last": null keeps every video`)
	if !ok {
		return
	}
	ch := FollowChange{MaxHeight: in.MaxHeight, IncludeShorts: in.IncludeShorts, Enabled: in.Enabled, Mirror: in.Mirror}
	if _, set := keys["keep_last"]; set {
		ch.KeepLast = &in.KeepLast
	}
	f, err := h.Svc.UpdateFollow(r.Context(), c.UserID, id, ch)
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, f)
}

func (h *Handlers) unfollow(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var del bool
	switch r.URL.Query().Get("files") {
	case "delete":
		del = true
	case "keep":
	default:
		webserver.Fail(w, http.StatusBadRequest, "say files=keep or files=delete")
		return
	}
	if err := h.Svc.Unfollow(r.Context(), c.UserID, id, del); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// videoPath is a downloaded video's file under the YouTube folder. False for
// a video not done, no folder, or a path that leaves the folder.
func videoPath(root string, v Video) (string, bool) {
	if root == "" || v.State != StateDone || v.RelativePath == "" {
		return "", false
	}
	root = filepath.Clean(root)
	file := filepath.Join(root, filepath.FromSlash(v.RelativePath))
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) {
		return "", false
	}
	return file, true
}

// serveVideoFile sends one of the caller's downloaded videos, or with thumb
// its thumbnail. Anything else, someone else's video included, is 404.
func (h *Handlers) serveVideoFile(w http.ResponseWriter, r *http.Request, thumb bool) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := VideoOf(r.Context(), h.Svc.Store.Read(), c.UserID, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	file, ok := videoPath(LoadSettings(r.Context(), h.Svc.Store.Read()).Folder, v)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	// Named outright: the type must not hang on the server's mime.types file
	// or on sniffing, or Safari will not play the video.
	if thumb {
		w.Header().Set("Content-Type", "image/jpeg")
		webserver.ServeAttachment(w, r, thumbOf(file), filepath.Base(thumbOf(file)), false)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	webserver.ServeAttachment(w, r, file, filepath.Base(file), r.URL.Query().Get("download") == "1")
}

func (h *Handlers) videoFile(w http.ResponseWriter, r *http.Request) { h.serveVideoFile(w, r, false) }

func (h *Handlers) videoThumb(w http.ResponseWriter, r *http.Request) { h.serveVideoFile(w, r, true) }

func (h *Handlers) savePosition(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Seconds  int `json:"seconds"`
		Duration int `json:"duration"`
	}
	if !webserver.Decode(w, r, &in, `send {"seconds": 120, "duration": 600}`) {
		return
	}
	done, err := h.Svc.SavePosition(r.Context(), c.UserID, id, in.Seconds, in.Duration)
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]bool{"watched": done})
}

func (h *Handlers) retryVideo(w http.ResponseWriter, r *http.Request) {
	c, ok := h.caller(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := h.Svc.RetryVideo(r.Context(), c.UserID, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}
