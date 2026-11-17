package main

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// artwork serves a stored Library picture (discover.LibraryArtwork).
func (a *handlers) artwork(w http.ResponseWriter, r *http.Request) {
	discover.LibraryArtwork(w, r, a.s.Library.Artwork)
}

// tmdbArtwork serves a TMDB picture through the artwork cache (discover.TMDBArtwork).
func (a *handlers) tmdbArtwork(w http.ResponseWriter, r *http.Request) {
	var c *library.ArtworkCache
	if a.s.Library != nil {
		c = a.s.Library.Artwork
	}
	discover.TMDBArtwork(w, r, c, &a.artMisses)
}

// serveRemoteArtwork is discover.ServeRemoteArtwork on the Library's cache.
func (a *handlers) serveRemoteArtwork(w http.ResponseWriter, r *http.Request, src string, send func(http.ResponseWriter, *http.Request, string, string)) {
	discover.ServeRemoteArtwork(w, r, a.s.Library.Artwork, &a.artMisses, src, send)
}

func (a *handlers) serverArtLink(u string) string {
	if a.s.MetadataModes.Direct() && metadata.FanartPicture(u) {
		return u // fanart.tv with the owner's own key (Your own keys)
	}
	return discover.ServerArtLink(a.s.ImageBase, u)
}

func (a *handlers) serverArtwork(w http.ResponseWriter, r *http.Request) {
	var c *library.ArtworkCache
	if a.s.Library != nil {
		c = a.s.Library.Artwork
	}
	discover.ServerArtwork(w, r, a.s.ImageBase, c, &a.artMisses)
}

func (a *handlers) downloadPaths(r *http.Request) []string {
	ps, _ := library.DownloadPaths(r.Context(), a.s.Store.Read(), a.s.Library.SavePath)
	return ps
}

// titleHandlers build the title routes on this API's services and glue.
func (a *handlers) titleHandlers() *discover.TitleHandlers {
	return &discover.TitleHandlers{Store: a.s.Store, Library: a.s.Library, Metadata: a.s.Metadata, Queue: a.s.Queue, Asks: a.s.Asks,
		Live: a.s.Live != nil, Actor: actor, Person: a.person, Fail: library.WriteError, ShowError: showError,
		KindDefaultsFor: a.kindDefaultsFor, TitleID: a.titleID, TitleView: a.titleView, PublishTitleView: a.tv().PublishView,
		KidMax: a.kidMax, KidLimited: a.disc.KidLimited, KidMovies: a.kidMovies, OpenAsk: a.openAsk, OpenMovieAsk: a.openMovieAsk}
}
