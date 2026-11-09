package setup

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// actor is who asks (the owner without sign-in).
func actor(r *http.Request) domain.Actor {
	return webserver.ActorOr(r, domain.Actor{Kind: "user", ID: 1})
}

// Routes mounts the part switches (setup) and the owner and admin
// controls (accounts) (3.7.6 and 3.7.7).
func (h *Handlers) Routes(m *http.ServeMux) {
	confirm := func(w http.ResponseWriter, r *http.Request, what string) bool {
		return h.confirm.Confirmed(w, r, actor(r).ID, what)
	}
	(&PartHandlers{Parts: h.Parts, SafeMode: h.SafeMode, Actor: actor, Confirm: confirm, Event: h.ControlEvent}).Routes(m)
	(&accounts.ControlHandlers{Parts: h.Parts, States: h.States, ExitApp: h.ExitApp, Confirm: confirm, Event: h.ControlEvent,
		LinkTest: h.linkTest}).Routes(m)
}

// ConfirmCount is how many two-step confirmation ids are kept.
func (h *Handlers) ConfirmCount() int { return h.confirm.Len() }

// linkTest is the test route a link restart runs for kind; unavailable
// is set when that kind is not available here.
func (h *Handlers) linkTest(kind string) (test http.HandlerFunc, unavailable string) {
	switch kind {
	case "downloader":
		return h.TestDownloader, ""
	case "source":
		return h.TestSource, ""
	case "media_server":
		if h.TestMediaServer == nil {
			return nil, "media servers are not available here"
		}
		return h.TestMediaServer, ""
	case "subtitle_site":
		return h.TestSubtitleSite, ""
	}
	return nil, ""
}

// PartGate answers 409 "this part is off" for a route of a part that is
// off, before the route's own checks. Core routes always pass.
func (h *Handlers) PartGate(m *http.ServeMux, next http.Handler) http.Handler {
	if h.Parts == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := m.Handler(r)
		o, ok := parts.RouteOwner(pattern)
		if !ok || o == parts.Core || (h.Parts.SafeMode && parts.SafeLibrary[pattern]) {
			next.ServeHTTP(w, r)
			return
		}
		if o == parts.ByTitle {
			if p, found := parts.TitlePart(r.Context(), h.Store.Read(), pathValue(pattern, r.URL.Path, "{id}")); found {
				o = parts.Of(p)
			} else {
				next.ServeHTTP(w, r) // no such title: the route answers 404
				return
			}
		}
		if !h.Parts.OwnerOn(o) {
			webserver.WriteJSON(w, http.StatusConflict, PartOffBody(o))
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

// whoName names the person for an Event summary.
func whoName(r *http.Request) string {
	if who, ok := webserver.From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "The owner"
}

// ControlEvent writes the Event for a control: who, what and when.
func (h *Handlers) ControlEvent(r *http.Request, action, target, summary string) {
	data := map[string]any{"action": action, "target": target}
	if who, ok := webserver.From(r.Context()); ok {
		data["via"] = who.Via
	}
	_ = h.Store.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: actor(r), Kind: domain.EventControl,
		Summary: whoName(r) + " " + summary, Data: data})
}

// ReportTitleGate is the same title check as GET /titles/{id} for a report:
// no such title and a title above a Kid's limit answer alike, and a title of
// a part that is off waits.
func (h *Handlers) ReportTitleGate(w http.ResponseWriter, r *http.Request, id int64) bool {
	var t mediafiles.Title
	if err := database.Get(r.Context(), h.Store.Read(), "titles", id, &t); err != nil || !h.TitleAllowed(r, t) {
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			webserver.Internal(w, "api", err)
			return false
		}
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return false
	}
	if p, known := parts.ForTitleKind(t.Kind); known && !parts.PartOn(h.Parts, p) {
		webserver.WriteJSON(w, http.StatusConflict, PartOffBody(parts.Of(p)))
		return false
	}
	return true
}
