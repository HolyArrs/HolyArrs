package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// User is one person as a plugin with requests.users.read sees them:
// an id and a display name, nothing else.
type User struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Handlers serves the provider-hook routes.
type Handlers struct {
	Reg *Registry
	// ValidScope says whether a scope name is known.
	ValidScope func(string) bool
	// Users lists the people (id and name only); nil answers 503.
	Users func(ctx context.Context) ([]User, error)
	// StreamFile is the path of a media file by id, when it is inside a
	// folder the owner granted that plugin; nil answers 503.
	StreamFile func(ctx context.Context, plugin string, fileID int64) (string, error)
	// AsPlugin runs r through the gated app routes as plugin, with its
	// scopes only (the plugin page bridge); nil answers 503.
	AsPlugin func(plugin string, r *http.Request) (*http.Response, error)
	// BridgeAllowed resolves the route r asks for and allows it only when
	// plugin holds its scope and who holds its own permission; nil refuses
	// every bridge call.
	BridgeAllowed func(plugin string, who webserver.Caller, r *http.Request) error

	// SetArtwork stores a plugin's picture as a title's poster or backdrop;
	// nil answers 503.
	SetArtwork func(ctx context.Context, titleID int64, kind string, pic []byte) error
	// Notify sends a plugin's message through the owner's alerts; nil
	// answers 503.
	Notify func(ctx context.Context, plugin, subject, text string) error

	pageToks *streams
	notified notifyLimit
}

const (
	pollWait  = 25 * time.Second
	answerMax = 20 << 20
)

// Routes registers the hook routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	h.pageToks = newStreams()
	// Plugin side: called with a plugin token only.
	m.HandleFunc("GET /api/v1/plugin-hooks/downloader", h.next(Downloader))
	m.HandleFunc("POST /api/v1/plugin-hooks/downloader", h.answer(Downloader))
	m.HandleFunc("GET /api/v1/plugin-hooks/releases", h.next(Releases))
	m.HandleFunc("POST /api/v1/plugin-hooks/releases", h.answer(Releases))
	m.HandleFunc("GET /api/v1/plugin-hooks/tools", h.next(Tools))
	m.HandleFunc("POST /api/v1/plugin-hooks/tools", h.answer(Tools))
	m.HandleFunc("PUT /api/v1/plugin-hooks/tools", h.declareTools)
	m.HandleFunc("GET /api/v1/plugin-hooks/events", h.next(Events))
	m.HandleFunc("GET /api/v1/plugin-hooks/sections", h.next(Sections))
	m.HandleFunc("POST /api/v1/plugin-hooks/sections", h.answer(Sections))
	m.HandleFunc("GET /api/v1/plugin-hooks/pages", h.next(Pages))
	m.HandleFunc("POST /api/v1/plugin-hooks/pages", h.answer(Pages))
	m.HandleFunc("PUT /api/v1/plugin-hooks/pages", h.declarePages)
	m.HandleFunc("PUT /api/v1/plugin-hooks/home-widget", h.declareWidget)
	m.HandleFunc("GET /api/v1/plugin-hooks/users", h.users)
	m.HandleFunc("POST /api/v1/plugin-hooks/stream", h.stream)
	m.HandleFunc("POST /api/v1/plugin-hooks/artwork", h.artwork)
	m.HandleFunc("POST /api/v1/plugin-hooks/notify", h.notify)
	m.HandleFunc("GET /api/v1/plugin-hooks/metadata", h.next(Metadata))
	m.HandleFunc("POST /api/v1/plugin-hooks/metadata", h.answer(Metadata))
	m.HandleFunc("GET /api/v1/plugin-hooks/botcheck", h.next(Botcheck))
	m.HandleFunc("POST /api/v1/plugin-hooks/botcheck", h.answer(Botcheck))
	// App side.
	m.HandleFunc("GET /api/v1/plugin-widgets", h.widgets)
	m.HandleFunc("GET /api/v1/titles/{id}/plugin-sections", h.sections)
	m.HandleFunc("GET /api/v1/plugin-pages", h.pages)
	m.HandleFunc("POST /api/v1/plugin-pages/{plugin}/{page}/open", h.openPage)
	m.HandleFunc("POST /api/v1/plugin-pages/{plugin}/call", h.bridge)
	m.HandleFunc("POST /api/v1/plugin-pages/{plugin}/play", h.play)
	m.HandleFunc("GET /api/v1/plugin-page-files/{token}/{path...}", h.pageFile)
	m.HandleFunc("GET /api/v1/plugin-stream/{token}", h.streamFile)
}

// pluginWith is plugin, refusing one that doesn't hold scope right now.
func (h *Handlers) pluginWith(w http.ResponseWriter, r *http.Request, scope string) (string, bool) {
	id, ok := plugin(w, r)
	if ok && !h.Reg.granted(id, scope) {
		webserver.Fail(w, http.StatusForbidden, "this plugin was not given the "+scope+" permission")
		return "", false
	}
	return id, ok
}

// plugin is the calling plugin's id; anyone else is refused.
func plugin(w http.ResponseWriter, r *http.Request) (string, bool) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "plugin" || who.PluginID == "" {
		webserver.Fail(w, http.StatusForbidden, "only a running plugin calls this")
		return "", false
	}
	return who.PluginID, true
}

func hookFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoProvider):
		webserver.Fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrDropped):
		webserver.Fail(w, http.StatusGone, err.Error())
	default:
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	}
}

func (h *Handlers) next(hook string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := h.pluginWith(w, r, Scopes[hook])
		if !ok {
			return
		}
		j, got, err := h.Reg.Broker.Next(r.Context(), id, hook, pollWait)
		if err != nil {
			hookFail(w, err)
			return
		}
		if !got {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		webserver.WriteJSONNoStore(w, http.StatusOK, j)
	}
}

func (h *Handlers) answer(hook string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := h.pluginWith(w, r, Scopes[hook])
		if !ok {
			return
		}
		var in struct {
			Job string `json:"job"`
			Answer
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, answerMax)).Decode(&in); err != nil || in.Job == "" {
			webserver.Fail(w, http.StatusBadRequest, `send {"job": "...", "data": ...} or {"job": "...", "error": "..."}`)
			return
		}
		if err := h.Reg.Broker.Answer(id, hook, in.Job, in.Answer); err != nil {
			webserver.Fail(w, http.StatusNotFound, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	b, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, "could not read the body")
		return nil, false
	}
	return b, true
}

// declare reads a plugin's declaration with check and keeps it with set.
func (h *Handlers) declare(w http.ResponseWriter, r *http.Request, scope string, read func([]byte) error, set func(string)) {
	id, ok := h.pluginWith(w, r, scope)
	if !ok {
		return
	}
	b, ok := readBody(w, r)
	if !ok {
		return
	}
	if err := read(b); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	set(id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) declareTools(w http.ResponseWriter, r *http.Request) {
	var v []Tool
	h.declare(w, r, Scopes[Tools], func(b []byte) (err error) { v, err = CheckTools(b, h.ValidScope); return },
		func(id string) { h.Reg.setTools(id, v) })
}

func (h *Handlers) declarePages(w http.ResponseWriter, r *http.Request) {
	var v []Page
	h.declare(w, r, Scopes[Pages], func(b []byte) (err error) { v, err = CheckPages(b); return },
		func(id string) { h.Reg.setPages(id, v) })
}

func (h *Handlers) declareWidget(w http.ResponseWriter, r *http.Request) {
	var v Card
	h.declare(w, r, "pages.home-widget", func(b []byte) (err error) { v, err = CheckCard(b); return },
		func(id string) { h.Reg.setWidget(id, v) })
}

func (h *Handlers) users(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.pluginWith(w, r, "requests.users.read"); !ok {
		return
	}
	if h.Users == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "not available")
		return
	}
	us, err := h.Users(r.Context())
	if err != nil {
		webserver.Fail(w, http.StatusInternalServerError, "could not read the people list")
		return
	}
	if us == nil {
		us = []User{}
	}
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"users": us})
}

func (h *Handlers) stream(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pluginWith(w, r, "media.stream")
	if !ok {
		return
	}
	var in struct {
		FileID int64 `json:"file_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil || in.FileID <= 0 {
		webserver.Fail(w, http.StatusBadRequest, `send {"file_id": 123}`)
		return
	}
	if h.StreamFile == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "not available")
		return
	}
	if _, err := h.StreamFile(r.Context(), id, in.FileID); err != nil {
		webserver.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	tok, until := h.Reg.streams.issue(id, strconv.FormatInt(in.FileID, 10))
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"url": "/api/v1/plugin-stream/" + tok, "expires": until})
}

// streamFile serves a stream link with range support. The token is random,
// short-lived and names one file by id; every request checks again that the
// plugin still holds media.stream and the file is still in its folders.
func (h *Handlers) streamFile(w http.ResponseWriter, r *http.Request) {
	pl, ref, ok := h.Reg.streams.open(r.PathValue("token"))
	fid, _ := strconv.ParseInt(ref, 10, 64)
	if !ok || fid <= 0 || h.StreamFile == nil || !h.Reg.granted(pl, "media.stream") || !mediaFetch(r) || !streamViewer(r) {
		webserver.Fail(w, http.StatusNotFound, "this link has ended")
		return
	}
	var alive context.Context
	if h.Reg.Alive != nil {
		alive = h.Reg.Alive(pl)
	}
	if alive == nil {
		webserver.Fail(w, http.StatusNotFound, "this link has ended")
		return
	}
	p, err := h.StreamFile(r.Context(), pl, fid)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "this link has ended")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(alive, cancel) // revoking the plugin ends the stream
	defer stop()
	serveMedia(&stoppable{ResponseWriter: w, ctx: ctx}, r.WithContext(ctx), p)
}

func (h *Handlers) widgets(w http.ResponseWriter, r *http.Request) {
	ws := h.Reg.Widgets()
	if ws == nil {
		ws = []Widget{}
	}
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"widgets": ws})
}

// sections asks every section plugin about one title, briefly; a slow or
// failing plugin is left out.
func (h *Handlers) sections(w http.ResponseWriter, r *http.Request) {
	tid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || tid <= 0 {
		webserver.Fail(w, http.StatusBadRequest, "bad title id")
		return
	}
	out := []Widget{}
	for _, p := range h.Reg.Broker.Providers(Sections) {
		raw, err := h.Reg.Broker.Call(r.Context(), p, Sections, "section", map[string]int64{"title_id": tid}, 2*time.Second)
		if err != nil || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		if c, err := CheckCard(raw); err == nil {
			out = append(out, Widget{Plugin: p, Card: c})
		}
	}
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"sections": out})
}

// mediaFetch says whether r is a media element or plain fetch of the file,
// not a page, frame, script or worker load, and comes from no other site's
// page (a sandboxed plugin page has the origin "null").
func mediaFetch(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "", "video", "audio", "track", "empty":
	default:
		return false
	}
	o := r.Header.Get("Origin")
	return o == "" || o == "null" || strings.EqualFold(strings.TrimPrefix(strings.TrimPrefix(o, "https://"), "http://"), r.Host)
}

// streamViewer: a stream plays for a person signed in to HolyArrs in this
// browser, reached directly (no proxy or forwarding plugin in between).
func streamViewer(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "session" || who.UserID == 0 {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded", "X-Holyarrs-Plugin-Proxy"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	return true
}

// stoppable refuses to write once ctx ends, so a revoked stream stops at
// the next chunk.
type stoppable struct {
	http.ResponseWriter
	ctx context.Context
}

func (s *stoppable) Write(b []byte) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	return s.ResponseWriter.Write(b)
}
