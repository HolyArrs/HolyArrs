package api

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// decode reads a JSON body into v; false means an answer was already sent.
func decode(w http.ResponseWriter, r *http.Request, v any, help string) bool {
	return webserver.Decode(w, r, v, help)
}

// libraryError answers the library's refusals (library.WriteError).
func libraryError(w http.ResponseWriter, err error) { library.WriteError(w, err) }

// metadataServiceError is library.ServiceError.
func metadataServiceError(w http.ResponseWriter, err error) bool { return library.ServiceError(w, err) }

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	return webserver.PathID(w, r, "id")
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
