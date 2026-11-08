package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// showRoutes serves adding shows, their seasons and episodes, and the show
// settings.
func (a *API) showRoutes(m *http.ServeMux) {
	(&shows.AddHandlers{Store: a.s.Store, Lib: a.s.Library, Fail: showError, KidLimited: a.kidLimited,
		KidShows: func(r *http.Request, list []metadata.Show) []metadata.Show { return a.kidShows(r.Context(), a.kidMax(r), list) },
		OpenAsk:  a.showAskFor, Defaults: a.showDefaults, Follow: a.followAdded, Added: a.addedTitle}).Routes(m)
	(&shows.EpisodeHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Fail: showError, StateLabels: StateLabels, Trim: trimEpisodeSlots}).Routes(m)
	(&shows.SeasonHandlers{Store: a.s.Store, Lib: a.s.Library, TitleID: a.titleID, Fail: showError}).Routes(m)
}

// showAskFor is the person's open ask for a show, nil when none.
func (a *API) showAskFor(r *http.Request, ids map[string]string) (any, error) {
	b, err := a.openShowAsk(r.Context(), ids, a.person(r).UserID)
	if b == nil || err != nil {
		return nil, err
	}
	return b, nil
}

// showDefaults fills a show add's left-out options from the saved TV defaults.
func (a *API) showDefaults(ctx context.Context, sent map[string]json.RawMessage, o *shows.ShowAddOptions) error {
	return a.kindDefaultsFor(ctx, "show", sent, &o.LibraryFolderID, &o.PickRulesID, &o.KeepUpToDate, &o.SearchOnAdd)
}

// followAdded puts an added Title on the adder's calendar; it does nothing
// when asks are off.
func (a *API) followAdded(r *http.Request, titleID int64) error {
	if a.s.Asks == nil {
		return nil
	}
	return a.s.Asks.FollowAdded(r.Context(), a.person(r), titleID)
}

// addedTitle is the title view of a just added Title, also sent to live pages.
func (a *API) addedTitle(ctx context.Context, t mediafiles.Title) (any, error) {
	v, err := a.titleView(ctx, t)
	if err != nil {
		return nil, err
	}
	if a.s.Live != nil {
		a.publishTitleView(v)
	}
	return v, nil
}

// showSource is the named show details source.
func (a *API) showSource(name string) (metadata.ShowProvider, error) {
	return shows.Source(a.s.Library, name)
}

// showError answers show refusals with a status and a reason; anything
// else goes to libraryError.
func showError(w http.ResponseWriter, err error) {
	var dup *shows.ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, library.ErrNoShowFolder), errors.Is(err, shows.ErrShowNeverAdd):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadSeasonChoice), errors.Is(err, library.ErrBadNewSeasons), errors.Is(err, library.ErrBadOrder),
		errors.Is(err, library.ErrBadWait), errors.Is(err, library.ErrNotShowFolder), errors.Is(err, library.ErrMovieOnly),
		errors.Is(err, library.ErrShowOnly), errors.Is(err, metadata.ErrUnknownSource):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrNotShow), errors.Is(err, library.ErrNoSuchSeason), errors.Is(err, metadata.ErrShowNotFound):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, metadata.ErrNoTVDBKey), errors.Is(err, library.ErrNoShowSources):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		libraryError(w, err)
	}
}

// EpisodeSlot and EpisodeView are the shows module's episode views.
type (
	EpisodeSlot = shows.EpisodeSlot
	EpisodeView = shows.EpisodeView
)
