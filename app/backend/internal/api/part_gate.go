package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	setupmod "github.com/HolyArrs/holyarrs/app/backend/internal/setup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// controlRoutes mounts the part switches (setup) and the owner and admin
// controls (accounts) (3.7.6 and 3.7.7).
func (a *API) controlRoutes(m *http.ServeMux) {
	confirm := func(w http.ResponseWriter, r *http.Request, what string) bool {
		return a.confirm.Confirmed(w, r, actor(r).ID, what)
	}
	(&setupmod.PartHandlers{Parts: a.s.Parts, SafeMode: a.s.SafeMode, Actor: actor, Confirm: confirm, Event: a.controlEvent}).Routes(m)
	(&accounts.ControlHandlers{Parts: a.s.Parts, States: a.s.States, ExitApp: a.s.ExitApp, Confirm: confirm, Event: a.controlEvent,
		LinkTest: a.linkTest}).Routes(m)
}

// linkTest is the test route a link restart runs for kind; unavailable
// is set when that kind is not available here.
func (a *API) linkTest(kind string) (test http.HandlerFunc, unavailable string) {
	switch kind {
	case "downloader":
		return a.dl.TestDownloader, ""
	case "source":
		return a.sourceHandlers().TestSource, ""
	case "media_server":
		if a.s.Media == nil {
			return nil, "media servers are not available here"
		}
		return a.media.Test, ""
	case "subtitle_site":
		return subtitles.NewHandlers(a.s.Store, a.s.Secrets, a.s.Subtitles).ServeSiteTest, ""
	}
	return nil, ""
}

// partOffBody is what a route of a part that is off answers.
func partOffBody(o parts.Owner) map[string]string { return setupmod.PartOffBody(o) }

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
			webserver.WriteJSON(w, http.StatusConflict, partOffBody(o))
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

// reportTitleGate is the same title check as GET /titles/{id} for a report:
// no such title and a title above a Kid's limit answer alike, and a title of
// a part that is off waits.
func (a *API) reportTitleGate(w http.ResponseWriter, r *http.Request, id int64) bool {
	var t mediafiles.Title
	if err := database.Get(r.Context(), a.s.Store.Read(), "titles", id, &t); err != nil || !a.titleAllowed(r, t) {
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			webserver.Internal(w, "api", err)
			return false
		}
		webserver.Fail(w, http.StatusNotFound, "no such Title")
		return false
	}
	if p, known := parts.ForTitleKind(t.Kind); known && !a.partIsOn(p) {
		webserver.WriteJSON(w, http.StatusConflict, partOffBody(parts.Of(p)))
		return false
	}
	return true
}

// followsChanged drops a person's music rows built from their follows.
func (a *API) followsChanged(userID int64) {
	a.musicv2.days.Invalidate(userID)
	a.forgetMusicV7Snaps(userID)
}

// viewer is the signed-in person's id (1, the owner, without sign-in).
func viewer(ctx context.Context) int64 {
	if who, ok := webserver.From(ctx); ok {
		return who.UserID
	}
	return 1
}
