package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// The ask routes live in requests (AskHandlers); these are the pieces the
// title, show, music and discover pages here still read.

type askBrief = requests.AskBrief

// person is the signed-in person as Requests sees them. Without
// sign-in (tests) it is user 1 with every right.
func (a *API) person(r *http.Request) requests.Person {
	who, ok := webserver.From(r.Context())
	if !ok {
		var name string
		_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT name FROM users WHERE id = 1").Scan(&name)
		return requests.Person{UserID: 1, Name: name, AutoApprove: true, NoLimit: true}
	}
	return requests.Person{UserID: who.UserID, Name: who.Name, AutoApprove: who.Has(webserver.PermAutoApprove), NoLimit: who.Has(webserver.PermApprove),
		MaxRating: a.kidMax(r)}
}

// mayApprove says whether the signed-in person decides asks.
func mayApprove(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return !ok || who.Has(webserver.PermApprove)
}

func (a *API) askHandlers() *requests.AskHandlers {
	return &requests.AskHandlers{Svc: a.s.Asks, Store: a.s.Store, Person: a.person, Actor: actor,
		PartOff: func(w http.ResponseWriter, kind string) bool {
			if p, known := parts.ForTitleKind(kind); known && !a.partIsOn(p) {
				webserver.WriteJSON(w, http.StatusConflict, partOffBody(parts.Of(p)))
				return true
			}
			return false
		},
		SeenArtists: func(vs []requests.View) {
			var names []nameKey
			for _, v := range vs {
				if v.Kind == "artist" && v.ArtistID != "" {
					names = append(names, nameKey{kind: "artist", mbid: strings.ToLower(v.ArtistID), name: v.Name})
				}
			}
			a.rememberMusicNames(names)
		},
		MusicError: musicError, ShowError: showError}
}

func (a *API) openAsk(ctx context.Context, tmdbID string, me int64) (*askBrief, error) {
	return requests.OpenMovieAsk(ctx, a.s.Store.Read(), tmdbID, me, false)
}

func (a *API) openMovieAsk(ctx context.Context, tmdbID string, me int64, fourK bool) (*askBrief, error) {
	return requests.OpenMovieAsk(ctx, a.s.Store.Read(), tmdbID, me, fourK)
}

func (a *API) openShowAsk(ctx context.Context, ids map[string]string, me int64) (*askBrief, error) {
	return requests.OpenShowAsk(ctx, a.s.Store.Read(), ids, me)
}

func (a *API) openArtistAsks(ctx context.Context, mbid string, me int64) ([]askBrief, error) {
	return requests.OpenArtistAsks(ctx, a.s.Store.Read(), mbid, me)
}
