package shows

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// showDefaults fills a show add's left-out options from the saved TV defaults.
func (h *Handlers) showDefaults(ctx context.Context, sent map[string]json.RawMessage, o *ShowAddOptions) error {
	return h.KindDefaultsFor(ctx, "show", sent, &o.LibraryFolderID, &o.PickRulesID, &o.KeepUpToDate, &o.SearchOnAdd)
}

// ShowSource is the named show details source.
func (h *Handlers) ShowSource(name string) (metadata.ShowProvider, error) {
	return Source(h.Library, name)
}

// Error answers show refusals with a status and a reason; anything else
// goes to LibraryError.
func (h *Handlers) Error(w http.ResponseWriter, err error) {
	ShowError(w, err, h.LibraryError)
}

// ShowError answers show refusals with a status and a reason; anything
// else goes to other.
func ShowError(w http.ResponseWriter, err error, other func(http.ResponseWriter, error)) {
	var dup *ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, library.ErrNoShowFolder), errors.Is(err, ErrShowNeverAdd):
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
		other(w, err)
	}
}
