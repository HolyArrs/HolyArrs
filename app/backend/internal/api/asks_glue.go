package api

import (
	"context"
	"encoding/json"
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
	return requests.PersonOf(r, a.kidMax, a.ownerName)
}

// ownerName is user 1's name (the stand-in person without sign-in).
func (a *API) ownerName(ctx context.Context) string {
	var name string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT name FROM users WHERE id = 1").Scan(&name)
	return name
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

// decodeKeys is decode that also answers which top-level keys the body
// holds, so a left-out field can take a saved default.
func decodeKeys(w http.ResponseWriter, r *http.Request, v any, help string) (map[string]json.RawMessage, bool) {
	return webserver.DecodeKeys(w, r, v, help)
}

// kindDefaultsFor fills an add's left-out fields from the saved defaults
// (requests.KindDefaultsFor).
func (a *API) kindDefaultsFor(ctx context.Context, kind string, sent map[string]json.RawMessage,
	folder, rules *int64, keep **bool, search *bool) error {
	return requests.KindDefaultsFor(ctx, a.s.Store.Read(), kind, sent, folder, rules, keep, search)
}
