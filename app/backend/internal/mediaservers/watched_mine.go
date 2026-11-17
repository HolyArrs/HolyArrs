package mediaservers

import (
	"context"
	"net/http"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// MyWatched is what one person watched on the media servers, for the
// Library's Watched marks: only their own linked accounts, never anybody
// else's. One answer covers the whole Library, so no card asks on its own.
type MyWatched struct {
	// Enabled says the owner keeps watch history on.
	Enabled bool `json:"enabled"`
	// Linked says the person has an account on a media server here; a
	// person who signs in with a password only has none.
	Linked bool `json:"linked"`
	// Movies are the ids of the movies watched.
	Movies []int64 `json:"movies"`
	// Shows count the episodes watched, by show id and season number.
	Shows map[int64]map[int]int `json:"shows"`
}

// WatchedBy answers what the person watched, from the last watch history
// read. An episode seen on two servers counts once.
func WatchedBy(ctx context.Context, q watchReader, userID int64) (MyWatched, error) {
	out := MyWatched{Enabled: WatchHistoryOn(ctx, q), Movies: []int64{}, Shows: map[int64]map[int]int{}}
	if !out.Enabled || userID == 0 {
		return out, nil
	}
	mine, err := accountsOf(ctx, q, userID)
	if err != nil {
		return out, err
	}
	out.Linked = len(mine) > 0
	type ep struct {
		title           int64
		season, episode int
	}
	got := map[ep]bool{}
	for _, a := range mine {
		err := eachWatchedBy(ctx, q, a, func(title int64, season, episode int) {
			k := ep{title, season, episode}
			if got[k] {
				return
			}
			got[k] = true
			if season == 0 && episode == 0 {
				out.Movies = append(out.Movies, title)
				return
			}
			if out.Shows[title] == nil {
				out.Shows[title] = map[int]int{}
			}
			out.Shows[title][season]++
		})
		if err != nil {
			return out, err
		}
	}
	slices.Sort(out.Movies)
	return out, nil
}

// accountsOf lists the person's accounts on the media servers here: each
// link on every server of its kind, and a Plex server's account 1 (its
// owner, whose link keeps a plex.tv id instead) when the person is the one
// Plex link with that name.
func accountsOf(ctx context.Context, q watchReader, userID int64) ([]watchAccount, error) {
	links, err := accounts.LinksOfPerson(ctx, q, userID)
	if err != nil || len(links) == 0 {
		return nil, err
	}
	ms, err := listServers(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []watchAccount
	var plex map[string]accounts.LinkedPerson
	for _, m := range ms {
		linked := false
		for _, l := range links {
			if l.Provider == m.Kind {
				linked = true
				out = append(out, watchAccount{m.ID, l.ExtID})
			}
		}
		if !linked || m.Kind != "plex" {
			continue
		}
		if plex == nil {
			if plex, err = accounts.LinkedPeople(ctx, q, "plex"); err != nil {
				return nil, err
			}
		}
		owner := watchAccount{m.ID, "1"}
		if _, taken := plex[owner.key]; taken {
			continue
		}
		name, err := accountName(ctx, q, owner)
		if err != nil {
			return nil, err
		}
		if lp, ok := byPlexName(plex, name); ok && lp.ID == userID {
			out = append(out, owner)
		}
	}
	return out, nil
}

// AnyoneWatched is every title anybody watched on the media servers, for
// the owner's and admins' "Watched by anyone" Library filter.
type AnyoneWatched struct {
	Enabled bool    `json:"enabled"`
	Titles  []int64 `json:"titles"`
}

// watchedByAnyone answers the titles anybody watched.
func (h *Handlers) watchedByAnyone(w http.ResponseWriter, r *http.Request) {
	out, err := WatchedByAnyone(r.Context(), h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// myWatched answers what the signed-in person watched. Who is read from
// the sign-in only, never from the request.
func (h *Handlers) myWatched(w http.ResponseWriter, r *http.Request) {
	who, _ := webserver.From(r.Context())
	out, err := WatchedBy(r.Context(), h.Store.Read(), who.UserID)
	if err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}
