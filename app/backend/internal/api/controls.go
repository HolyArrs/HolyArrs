package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// controlRoutes are the part switches and the owner and admin controls
// (3.7.6 and 3.7.7). The controls need the owner or an admin and, for an
// API key, the control scope (accounts.RouteScopes).
func (a *API) controlRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/parts", a.listParts)
	m.HandleFunc("POST /api/v1/parts/{part}/on", a.partOn)
	m.HandleFunc("POST /api/v1/parts/{part}/off", a.partOff)
	m.HandleFunc("POST /api/v1/parts/{part}/restart", a.partRestart)
	m.HandleFunc("POST /api/v1/stages/{name}/pause", a.stageControl("pause"))
	m.HandleFunc("POST /api/v1/stages/{name}/resume", a.stageControl("resume"))
	m.HandleFunc("POST /api/v1/stages/{name}/restart", a.stageControl("restart"))
	m.HandleFunc("POST /api/v1/links/{kind}/{id}/restart", a.linkRestart)
	m.HandleFunc("POST /api/v1/app/restart", a.appExit("restart"))
	m.HandleFunc("POST /api/v1/app/safe-mode", a.appExit("safe_mode"))
	m.HandleFunc("POST /api/v1/app/shutdown", a.appExit("shutdown"))
}

// partOffBody is what a route of a part that is off answers.
func partOffBody(o parts.Owner) map[string]string {
	name := "This part"
	if o == parts.Media {
		name = "Movies, TV shows and Music are all off, so this"
	} else if parts.Valid(parts.Part(o)) {
		name = parts.Part(o).Label()
	}
	return map[string]string{"error": "this part is off", "part": string(o),
		"text": name + " is off. The owner or an admin can turn it on again; its data is kept."}
}

// partGate answers 409 "this part is off" for a route of a part that is
// off, before the route's own checks. Core routes always pass.
func (a *API) partGate(m *http.ServeMux, next http.Handler) http.Handler {
	if a.s.Parts == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := m.Handler(r)
		o, ok := parts.RouteOwner(pattern)
		if !ok || o == parts.Core || (a.s.Parts.SafeMode && parts.SafeLibrary[pattern]) {
			next.ServeHTTP(w, r)
			return
		}
		if o == parts.ByTitle {
			if p, found := parts.TitlePart(r.Context(), a.s.Store.Read(), pathValue(pattern, r.URL.Path, "{id}")); found {
				o = parts.Of(p)
			} else {
				next.ServeHTTP(w, r) // no such title: the route answers 404
				return
			}
		}
		if !a.s.Parts.OwnerOn(o) {
			writeJSON(w, http.StatusConflict, partOffBody(o))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pathValue picks the path segment that stands where name is in pattern.
func pathValue(pattern, path, name string) string {
	_, pat, _ := strings.Cut(pattern, " ")
	ps, xs := strings.Split(pat, "/"), strings.Split(path, "/")
	for i, seg := range ps {
		if seg == name && i < len(xs) {
			return xs[i]
		}
	}
	return ""
}

// partIsOn reports whether the part p is on (always, without a Control).
// In safe mode the Library still lists every title (parts.SafeLibrary).
func (a *API) partIsOn(p parts.Part) bool { return a.s.Parts == nil || a.s.Parts.On(p) }

// PartView is one part as the controls show it.
type PartView struct {
	Part   parts.Part `json:"part"`
	Name   string     `json:"name"`
	On     bool       `json:"on"`
	State  string     `json:"state"`
	Reason string     `json:"reason,omitempty"`
}

func (a *API) listParts(w http.ResponseWriter, r *http.Request) {
	out := []PartView{}
	if a.s.Parts == nil {
		for _, p := range parts.All() {
			out = append(out, PartView{Part: p, Name: p.Label(), On: true, State: "running"})
		}
	} else {
		all := parts.All()
		for i, st := range a.s.Parts.Statuses() {
			p := all[i]
			out = append(out, PartView{Part: p, Name: st.Name, On: a.s.Parts.On(p), State: string(st.State), Reason: st.Reason})
		}
	}
	safeScreens, stages := []string{}, []string{}
	if a.s.Parts != nil {
		if a.s.Parts.SafeMode {
			safeScreens = parts.SafeScreens
		}
		stages = a.s.Parts.Stages()
	}
	writeJSON(w, http.StatusOK, map[string]any{"parts": out, "screens": parts.Screens(), "safe_mode": a.s.SafeMode, "safe_screens": safeScreens, "stages": stages})
}

// confirms holds the confirmation ids of the two-step controls.
type confirms struct {
	mu  sync.Mutex
	ids map[string]confirmFor
}

type confirmFor struct {
	what    string // method and path
	user    int64
	expires time.Time
}

// maxConfirmsPerUser caps the open confirmation ids of one person.
const maxConfirmsPerUser = 100

// ConfirmFor is how long a confirmation id stays good.
const ConfirmFor = 60 * time.Second

// controlNow is the clock of the confirmation ids (tests move it).
var controlNow = time.Now

// confirmed runs the two-step confirm: without a confirm id in the body
// it answers 202 with a new id (valid ConfirmFor) and returns false; with
// a good id for the same person and request it returns true. what says in
// plain words what the second request will do.
func (a *API) confirmed(w http.ResponseWriter, r *http.Request, what string) bool {
	var body struct {
		Confirm string `json:"confirm"`
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<10))
	if err != nil || (len(strings.TrimSpace(string(raw))) > 0 && json.Unmarshal(raw, &body) != nil) {
		fail(w, http.StatusBadRequest, `send {} to ask, then {"confirm": "<id>"} within 60 seconds`)
		return false
	}
	key := r.Method + " " + r.URL.Path
	user := actor(r).ID
	now := controlNow()
	a.confirm.mu.Lock()
	defer a.confirm.mu.Unlock()
	if a.confirm.ids == nil {
		a.confirm.ids = map[string]confirmFor{}
	}
	for id, c := range a.confirm.ids {
		if now.After(c.expires) {
			delete(a.confirm.ids, id)
		}
	}
	if body.Confirm == "" {
		// At most maxConfirmsPerUser open ids per person: the oldest goes.
		var oldest string
		n := 0
		for id, c := range a.confirm.ids {
			if c.user != user {
				continue
			}
			n++
			if oldest == "" || c.expires.Before(a.confirm.ids[oldest].expires) {
				oldest = id
			}
		}
		if n >= maxConfirmsPerUser {
			delete(a.confirm.ids, oldest)
		}
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		id := hex.EncodeToString(b)
		a.confirm.ids[id] = confirmFor{what: key, user: user, expires: now.Add(ConfirmFor)}
		writeJSON(w, http.StatusAccepted, map[string]any{"confirm": id, "expires_in_s": int(ConfirmFor.Seconds()),
			"text": "To " + what + ", send the same request again within 60 seconds with {\"confirm\": \"" + id + "\"}."})
		return false
	}
	c, ok := a.confirm.ids[body.Confirm]
	if !ok || c.what != key || c.user != user {
		fail(w, http.StatusConflict, "that confirmation id is unknown, used, expired or for another control; ask again")
		return false
	}
	delete(a.confirm.ids, body.Confirm)
	return true
}

// whoName names the person for an Event summary.
func whoName(r *http.Request) string {
	if who, ok := webserver.From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "The owner"
}

// controlEvent writes the Event for a control: who, what and when.
func (a *API) controlEvent(r *http.Request, action, target, summary string) {
	data := map[string]any{"action": action, "target": target}
	if who, ok := webserver.From(r.Context()); ok {
		data["via"] = who.Via
	}
	_ = a.s.Store.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: actor(r), Kind: domain.EventControl,
		Summary: whoName(r) + " " + summary, Data: data})
}

func (a *API) partFrom(w http.ResponseWriter, r *http.Request) (parts.Part, bool) {
	if a.s.Parts == nil {
		fail(w, http.StatusServiceUnavailable, "part switches are not available here")
		return "", false
	}
	p := parts.Part(r.PathValue("part"))
	if !parts.Valid(p) {
		fail(w, http.StatusNotFound, parts.ErrNoPart.Error())
		return "", false
	}
	return p, true
}

// controlError answers a refused control in plain words.
func controlError(w http.ResponseWriter, err error) {
	var ne parts.NeedsError
	var oe parts.OffError
	switch {
	case errors.As(err, &ne), errors.As(err, &oe), errors.Is(err, parts.ErrSafeMode), errors.Is(err, parts.ErrBusy):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, parts.ErrNoStage), errors.Is(err, parts.ErrNoPart):
		fail(w, http.StatusNotFound, err.Error())
	default:
		fail(w, http.StatusConflict, err.Error())
	}
}

func (a *API) partOn(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	p, ok := a.partFrom(w, r)
	if !ok {
		return
	}
	if err := a.s.Parts.Set(r.Context(), p, true, actor(r)); err != nil {
		controlError(w, err)
		return
	}
	a.controlEvent(r, "part_on", string(p), "turned "+p.Label()+" on")
	a.listParts(w, r)
}

func (a *API) partOff(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	p, ok := a.partFrom(w, r)
	if !ok || !a.confirmed(w, r, "turn "+p.Label()+" off (its screens hide, its scheduled work stops and its waiting Jobs wait; its data is kept)") {
		return
	}
	if err := a.s.Parts.Set(r.Context(), p, false, actor(r)); err != nil {
		controlError(w, err)
		return
	}
	a.controlEvent(r, "part_off", string(p), "turned "+p.Label()+" off")
	a.listParts(w, r)
}

func (a *API) partRestart(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	p, ok := a.partFrom(w, r)
	if !ok || !a.confirmed(w, r, "restart "+p.Label()+" (running Jobs finish first; nothing is cut off)") {
		return
	}
	left, err := a.s.Parts.Restart(r.Context(), p)
	if err != nil {
		controlError(w, err)
		return
	}
	a.controlEvent(r, "part_restart", string(p), "restarted "+p.Label())
	text := p.Label() + " restarted."
	if left > 0 {
		text += " Some Jobs were still running after the wait; they carry on and finish, nothing was cut off."
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": text, "still_running": left})
}

func (a *API) stageControl(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		if a.s.Parts == nil {
			fail(w, http.StatusServiceUnavailable, "stage controls are not available here")
			return
		}
		name := r.PathValue("name")
		var err error
		switch action {
		case "pause":
			err = a.s.Parts.Pause(name, whoName(r))
		case "resume":
			err = a.s.Parts.Resume(name)
		default:
			err = a.s.Parts.RestartStage(name)
		}
		if err != nil {
			controlError(w, err)
			return
		}
		past := map[string]string{"pause": "paused", "resume": "resumed", "restart": "restarted"}[action]
		a.controlEvent(r, "stage_"+action, name, past+" the stage "+name)
		out := map[string]any{"text": "The stage " + name + " was " + past + "."}
		if a.s.States != nil {
			if st, ok := a.s.States.Get("stage:" + name); ok {
				out["state"] = st
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// linkRestart reconnects one linked app: a Downloader is signed in again
// and checked, a Source is tested now, a media server or subtitle site is
// tested again. Each answers what its own test answers. It needs the
// settings permission, like the test routes it runs, and writes its Event
// only when the linked app exists (an unknown id answers 404 first).
func (a *API) linkRestart(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	if who, ok := webserver.From(r.Context()); ok && !who.Has(webserver.PermSettings) {
		fail(w, http.StatusForbidden, "your account may not do this; ask the owner")
		return
	}
	kind := r.PathValue("kind")
	var test http.HandlerFunc
	switch kind {
	case "downloader":
		test = a.testDownloader
	case "source":
		test = a.testSource
	case "media_server":
		if a.s.Media == nil {
			fail(w, http.StatusServiceUnavailable, "media servers are not available here")
			return
		}
		test = a.testMediaServer
	case "subtitle_site":
		test = subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).ServeSiteTest
	default:
		fail(w, http.StatusNotFound, "kind is downloader, source, media_server or subtitle_site")
		return
	}
	sw := &statusWriter{ResponseWriter: w}
	test(sw, r)
	if sw.code != http.StatusNotFound && sw.code != http.StatusBadRequest {
		a.controlEvent(r, "link_restart", kind+":"+r.PathValue("id"), "reconnected the "+strings.ReplaceAll(kind, "_", " ")+" "+r.PathValue("id"))
	}
}

// statusWriter remembers the status a handler answered with.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.code == 0 {
		s.code = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// appExit is the whole-app restart, restart in safe mode and shut down.
// The process stops cleanly; Docker's restart policy starts it again.
func (a *API) appExit(how string) http.HandlerFunc {
	what := map[string]string{
		"restart":   "restart HolyArrs",
		"safe_mode": "restart HolyArrs in safe mode (only the core starts, once)",
		"shutdown":  "shut HolyArrs down",
	}[how]
	return func(w http.ResponseWriter, r *http.Request) {
		if !webserver.RequireStrong(w, r) {
			return
		}
		if a.s.ExitApp == nil {
			fail(w, http.StatusServiceUnavailable, "this server cannot restart itself")
			return
		}
		if !a.confirmed(w, r, what) {
			return
		}
		text := map[string]string{
			"restart":   "HolyArrs stops now and Docker's restart policy starts it again in a few seconds, unless the policy is \"no\"; then start the container yourself.",
			"safe_mode": "HolyArrs stops now and starts again in safe mode through Docker's restart policy, unless the policy is \"no\"; then start the container yourself. The start after that is normal.",
			"shutdown":  "HolyArrs shuts down now. Docker's restart policy starts it again unless the policy is \"no\" (or the container is stopped by hand).",
		}[how]
		// ExitApp arranges the exit (and the safe-mode mark) and stops
		// the process shortly after, so the answer still reaches the page.
		if err := a.s.ExitApp(how); err != nil {
			fail(w, http.StatusInternalServerError, "could not arrange safe mode; nothing was restarted (the log says why)")
			return
		}
		a.controlEvent(r, "app_"+how, "app", map[string]string{"restart": "restarted HolyArrs", "safe_mode": "restarted HolyArrs in safe mode", "shutdown": "shut HolyArrs down"}[how])
		writeJSON(w, http.StatusAccepted, map[string]any{"text": text})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
