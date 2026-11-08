package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// decode reads a JSON body into v; false means an answer was already sent.
func decode(w http.ResponseWriter, r *http.Request, v any, help string) bool {
	return webserver.Decode(w, r, v, help)
}

// libraryError answers the library's refusals with a status and a reason.
func libraryError(w http.ResponseWriter, err error) {
	if metadataServiceError(w, err) {
		return
	}
	var dup *library.AlreadyAddedError
	var fe *library.FolderError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.As(err, &fe):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "health": fe.Health.State, "reason": fe.Health.Reason})
	case errors.Is(err, library.ErrNeverAdd), errors.Is(err, library.ErrNoFolder), errors.Is(err, library.ErrFolderInUse),
		errors.Is(err, library.ErrFolderTaken), errors.Is(err, library.ErrFolderName), errors.Is(err, library.ErrMainVersion), errors.Is(err, library.ErrVersionHasFiles), errors.Is(err, library.ErrVersionBusy),
		errors.Is(err, library.ErrVersionFolder), errors.Is(err, library.ErrProtectedCopy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadGate), errors.Is(err, library.ErrBadDelay), errors.Is(err, library.ErrNotMovieFolder), errors.Is(err, library.ErrNotShowFolder), errors.Is(err, library.ErrNotMusicFolder),
		errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotMusicRules), errors.Is(err, library.ErrNotVideoRules), errors.Is(err, library.ErrFolderPath), errors.Is(err, library.ErrFolderKind),
		errors.Is(err, library.ErrFolderAbsent), errors.Is(err, library.ErrNoMetadataIDs), errors.Is(err, library.ErrVersionLabel),
		errors.Is(err, library.ErrTooManyVersions), errors.Is(err, library.ErrCopyChoice):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, metadata.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no movie with that id")
	case errors.Is(err, metadata.ErrNoAccess):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		webserver.Internal(w, "api", err)
	}
}

// metadataServiceError keeps service failures useful without exposing transport
// or credential details. Direct provider errors retain their existing answers.
func metadataServiceError(w http.ResponseWriter, err error) bool {
	var message string
	status := http.StatusServiceUnavailable
	switch {
	case errors.Is(err, metadata.ErrServiceAccess):
		message = metadata.ErrServiceAccess.Error()
	case errors.Is(err, metadata.ErrServiceUnavailable):
		message = metadata.ErrServiceUnavailable.Error()
	case errors.Is(err, metadata.ErrServiceBusy):
		message = metadata.ErrServiceBusy.Error()
	case errors.Is(err, metadata.ErrServiceIncomplete):
		message = metadata.ErrServiceIncomplete.Error()
	case errors.Is(err, metadata.ErrServiceInvalid):
		status = http.StatusBadGateway
		message = metadata.ErrServiceInvalid.Error()
	default:
		return false
	}
	webserver.Fail(w, status, message)
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// artwork serves a stored Library picture (discover.LibraryArtwork).
func (a *API) artwork(w http.ResponseWriter, r *http.Request) {
	discover.LibraryArtwork(w, r, a.s.Library.Artwork)
}

// tmdbArtwork serves a TMDB picture through the artwork cache (discover.TMDBArtwork).
func (a *API) tmdbArtwork(w http.ResponseWriter, r *http.Request) {
	var c *library.ArtworkCache
	if a.s.Library != nil {
		c = a.s.Library.Artwork
	}
	discover.TMDBArtwork(w, r, c, &a.artMisses)
}

// serveRemoteArtwork is discover.ServeRemoteArtwork on the Library's cache.
func (a *API) serveRemoteArtwork(w http.ResponseWriter, r *http.Request, src string, send func(http.ResponseWriter, *http.Request, string, string)) {
	discover.ServeRemoteArtwork(w, r, a.s.Library.Artwork, &a.artMisses, src, send)
}

func (a *API) serverArtLink(u string) string { return discover.ServerArtLink(a.s.ImageBase, u) }

func (a *API) serverArtwork(w http.ResponseWriter, r *http.Request) {
	var c *library.ArtworkCache
	if a.s.Library != nil {
		c = a.s.Library.Artwork
	}
	discover.ServerArtwork(w, r, a.s.ImageBase, c, &a.artMisses)
}

func serveArtwork(w http.ResponseWriter, r *http.Request, name, p string) {
	library.ServeArtwork(w, r, name, p)
}

func serveArtworkCached(w http.ResponseWriter, r *http.Request, name, p, cacheControl string) {
	library.ServeArtworkCached(w, r, name, p, cacheControl)
}

func (a *API) downloadPaths(r *http.Request) []string {
	ps, _ := library.DownloadPaths(r.Context(), a.s.Store.Read(), a.s.Library.SavePath)
	return ps
}

// FolderView and NeverAddView are the library module's views.
type (
	FolderView   = library.FolderView
	NeverAddView = library.NeverAddView
)

// LookupView is a looked-up movie with what the Library knows of it.
type LookupView = discover.LookupView

// titleHandlers build the title routes on this API's services and glue.
func (a *API) titleHandlers() *discover.TitleHandlers {
	return &discover.TitleHandlers{Store: a.s.Store, Library: a.s.Library, Metadata: a.s.Metadata, Queue: a.s.Queue, Asks: a.s.Asks,
		Live: a.s.Live != nil, Actor: actor, Person: a.person, Fail: libraryError, ShowError: showError,
		KindDefaultsFor: a.kindDefaultsFor, TitleID: a.titleID, TitleView: a.titleView, PublishTitleView: a.publishTitleView,
		KidMax: a.kidMax, KidLimited: a.kidLimited, KidMovies: a.kidMovies, OpenAsk: a.openAsk, OpenMovieAsk: a.openMovieAsk}
}
