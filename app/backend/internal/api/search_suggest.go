package api

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// suggest wires the live search handlers (discover.Suggest) over this API;
// the Wiring phase moves it to cmd/holyarrs.
func (a *API) suggest() *discover.Suggest {
	return &discover.Suggest{
		LiveSearch: func() bool { return a.s.LiveSearch },
		MusicSuggest: func() func(ctx context.Context, q string) ([]metadata.MusicSuggestion, error) {
			return a.s.MusicSuggest
		},
		Metadata:   func() metadata.Provider { return a.s.Metadata },
		Store:      func() *database.Store { return a.s.Store },
		SafeMode:   func() bool { return a.s.Parts.SafeMode },
		Music:      func() discover.SuggestMusic { return a.mh() },
		ShowSource: func(name string) (metadata.ShowProvider, error) { return a.shows.ShowSource(name) },
		PartOn:     a.partIsOn, KidMax: a.kidMax, UserID: func(r *http.Request) int64 { return a.person(r).UserID },
		TitleAllowed: a.titleAllowed, TitleView: a.titleView, LibraryPeople: a.libraryPeople,
		OpenAsk: a.openAsk, OpenShowAsk: a.openShowAsk,
	}
}
