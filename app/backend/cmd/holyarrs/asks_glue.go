package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	setupmod "github.com/HolyArrs/holyarrs/app/backend/internal/setup"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// The ask routes live in requests (AskHandlers); these are the pieces the
// title, show, music and discover pages here still read.

type askBrief = requests.AskBrief

// person is the signed-in person as Requests sees them. Without
// sign-in (tests) it is user 1 with every right.
func (a *handlers) person(r *http.Request) requests.Person {
	return requests.PersonOf(r, a.kidMax, func(ctx context.Context) string { return requests.OwnerName(ctx, a.s.Store.Read()) })
}

func (a *handlers) askHandlers() *requests.AskHandlers {
	return &requests.AskHandlers{Svc: a.s.Asks, Store: a.s.Store, Person: a.person, Actor: actor,
		PartOff: func(w http.ResponseWriter, kind string) bool {
			if p, known := parts.ForTitleKind(kind); known && !a.partIsOn(p) {
				webserver.WriteJSON(w, http.StatusConflict, setupmod.PartOffBody(parts.Of(p)))
				return true
			}
			return false
		},
		SeenArtists: func(vs []requests.View) {
			var names []music.NameKey
			for _, v := range vs {
				if v.Kind == "artist" && v.ArtistID != "" {
					names = append(names, music.NewNameKey("artist", strings.ToLower(v.ArtistID), v.Name, ""))
				}
			}
			a.mh().RememberNames(names)
		},
		MusicError: musicError, ShowError: showError}
}

func (a *handlers) openAsk(ctx context.Context, tmdbID string, me int64) (*askBrief, error) {
	return requests.OpenMovieAsk(ctx, a.s.Store.Read(), tmdbID, me, false)
}

func (a *handlers) openMovieAsk(ctx context.Context, tmdbID string, me int64, fourK bool) (*askBrief, error) {
	return requests.OpenMovieAsk(ctx, a.s.Store.Read(), tmdbID, me, fourK)
}

func (a *handlers) openShowAsk(ctx context.Context, ids map[string]string, me int64) (*askBrief, error) {
	return requests.OpenShowAsk(ctx, a.s.Store.Read(), ids, me)
}

// kindDefaultsFor fills an add's left-out fields from the saved defaults
// (requests.KindDefaultsFor).
func (a *handlers) kindDefaultsFor(ctx context.Context, kind string, sent map[string]json.RawMessage,
	folder, rules *int64, keep **bool, search *bool) error {
	return requests.KindDefaultsFor(ctx, a.s.Store.Read(), kind, sent, folder, rules, keep, search)
}
