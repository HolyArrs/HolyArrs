package music

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// musicRoutes serves adding artists, their albums, Pressings and tracks,
// and the music settings (stage 2.3).
func (h *Handlers) musicRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/artists/lookup", h.lookupArtists)
	m.HandleFunc("POST /api/v1/artists", h.addArtist)
	m.HandleFunc("PUT /api/v1/titles/{id}/artist", h.setArtist)
	if h.TrackRoutes != nil {
		h.TrackRoutes(m)
	}
	m.HandleFunc("GET /api/v1/albums/{id}/cover", h.albumCoverArt)
}

// musicError answers music refusals with a status and a reason; anything
// else goes to LibraryError.
func (h *Handlers) musicError(w http.ResponseWriter, err error) { Error(w, err, h.LibraryError) }

// libraryHandlers serves albums and the music settings.
func (h *Handlers) libraryHandlers() LibraryHandlers {
	return LibraryHandlers{Store: h.Store, Library: h.lib(), ArtistID: h.artistID, Actor: actor, Fallback: h.LibraryError}
}

// ArtistResult is a looked-up artist (without its albums) with what the
// Library knows of it.
type ArtistResult struct {
	metadata.Artist
	TitleID  int64 `json:"title_id,omitempty"` // set when already in the Library
	NeverAdd bool  `json:"never_add,omitempty"`
	// Image is the artist's picture (the music picture route).
	Image string `json:"image,omitempty"`
	// Ask is the open ask for the artist the person is part of (the oldest),
	// else the oldest open ask, if any; Asks lists every open ask for it.
	Ask  *requests.AskBrief  `json:"ask,omitempty"`
	Asks []requests.AskBrief `json:"asks,omitempty"`
	// AskAlbums are the artist's albums to ask for (with ?albums=true and
	// an mbid: term), oldest first.
	AskAlbums []AskableAlbum `json:"ask_albums,omitempty"`
}

// AskableAlbum is one album of a looked-up artist as the ask dialog shows
// it.
type AskableAlbum struct {
	MBID           string   `json:"mbid"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	SecondaryTypes []string `json:"secondary_types"`
	ReleaseDate    string   `json:"release_date,omitempty"`
	// Kept says the artist is in the Library and keeps this album up to
	// date; LeftOut says the artist in the Library leaves its kind out.
	Kept    bool `json:"kept"`
	LeftOut bool `json:"left_out"`
}

// askableAlbums lists an artist's albums for asking, oldest first, with
// what the Library keeps of them.
func (h *Handlers) askableAlbums(ctx context.Context, titleID int64, albums []metadata.Album) ([]AskableAlbum, error) {
	byMBID := map[string]mediafiles.Part{}
	artistKeep := false
	if titleID != 0 {
		db := h.Store.Read()
		if err := db.QueryRowContext(ctx, "SELECT keep_up_to_date FROM titles WHERE id = ?", titleID).Scan(&artistKeep); err != nil {
			return nil, err
		}
		ps, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL", titleID)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			byMBID[p.SourceIDs["musicbrainz"]] = p
		}
	}
	out := make([]AskableAlbum, 0, len(albums))
	for _, al := range albums {
		v := AskableAlbum{MBID: al.MBID, Name: al.Name, Type: al.PrimaryType, SecondaryTypes: listOr(al.SecondaryTypes), ReleaseDate: al.ReleaseDate}
		if p, ok := byMBID[al.MBID]; ok {
			v.Kept = artistKeep && p.KeepUpToDate
			v.LeftOut = !p.Included
		}
		out = append(out, v)
	}
	slices.SortStableFunc(out, func(x, y AskableAlbum) int {
		return strings.Compare(x.ReleaseDate+x.MBID, y.ReleaseDate+y.MBID)
	})
	return out, nil
}

// maxLookupsEach is how many music lookups one person may have in flight.
const maxLookupsEach = 2

// inFlight counts work in flight per person.
type inFlight struct {
	mu sync.Mutex
	n  map[int64]int
}

// take counts one more for who unless they already have limit.
func (f *inFlight) take(who int64, limit int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n == nil {
		f.n = map[int64]int{}
	}
	if f.n[who] >= limit {
		return false
	}
	f.n[who]++
	return true
}

func (f *inFlight) give(who int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n[who]--; f.n[who] <= 0 {
		delete(f.n, who)
	}
}

// lookupArtists finds artists to add by name, or one by "mbid:<id>";
// ?albums=true with an id also lists its albums to ask for.
func (h *Handlers) lookupArtists(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" || len(term) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?term=<artist name> or an id such as mbid:5b11f4ce-a62d-471e-81fc-a69a8278c7da")
		return
	}
	if h.KidMax(r) != "" {
		// Music has no age ratings, so a Kid finds none.
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"artists": []ArtistResult{}, "source": "musicbrainz"})
		return
	}
	me := h.Person(r).UserID
	if !h.lookups.take(me, maxLookupsEach) {
		w.Header().Set("Retry-After", "5")
		webserver.Fail(w, http.StatusTooManyRequests, "you already have two music lookups running; wait for them to finish and try again")
		return
	}
	defer h.lookups.give(me)
	ctx := r.Context()
	if !mayApprove(r) {
		// Members wait behind the scheduled refresh and staff at MusicBrainz.
		ctx = metadata.MemberLookup(ctx)
	}
	found, err := h.lib().LookupArtists(ctx, term)
	if errors.Is(err, library.ErrSearchFilling) {
		// An unseen name: the search keeps running behind and is kept, so
		// the next ask is answered from the store.
		w.Header().Set("Retry-After", "2")
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"artists": []ArtistResult{}, "source": "musicbrainz", "pending": true})
		return
	}
	if err != nil {
		h.musicError(w, err)
		return
	}
	out := []ArtistResult{}
	db := h.Store.Read()
	var names []nameKey
	withAlbums := r.URL.Query().Get("albums") == "true"
	for _, ar := range found {
		albums := ar.Albums
		ar.Albums = nil
		v := ArtistResult{Artist: ar}
		if metadata.ValidMBID(ar.MBID) {
			id := strings.ToLower(ar.MBID)
			v.Image = artPath("artist", id)
			names = append(names, nameKey{kind: "artist", mbid: id, name: ar.Name})
		}
		if v.TitleID, err = library.ArtistByMBID(r.Context(), db, ar.MBID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.Asks, err = h.OpenArtistAsks(r.Context(), ar.MBID, me); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for i := range v.Asks {
			if v.Ask == nil || (v.Asks[i].Mine && !v.Ask.Mine) {
				v.Ask = &v.Asks[i]
			}
		}
		if withAlbums && albums != nil {
			if v.AskAlbums, err = h.askableAlbums(r.Context(), v.TitleID, albums); err != nil {
				webserver.Internal(w, "api", err)
				return
			}
		}
		if v.NeverAdd, err = library.ArtistOnNeverAdd(r.Context(), db, ar.MBID); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		out = append(out, v)
	}
	h.rememberMusicNames(names)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"artists": out, "source": "musicbrainz"})
}

const addArtistHelp = `send {"id": "mbid:<MusicBrainz artist id>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, albums (all, future, missing, latest, first, none), new_albums (all, new, none), album_types (album, ep, single, broadcast, other), secondary_types (studio, live, compilation, ...), release_statuses (official, promotion, bootleg, pseudo-release), follow`

func (h *Handlers) addArtist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
		library.ArtistAddOptions
		Follow *bool `json:"follow"` // put it on the adder's calendar (default true)
	}
	if !decode(w, r, &body, addArtistHelp) {
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		webserver.Fail(w, http.StatusBadRequest, addArtistHelp)
		return
	}
	id, jobID, err := h.lib().AddArtist(r.Context(), body.ID, body.ArtistAddOptions, actor(r))
	if err != nil {
		h.musicError(w, err)
		return
	}
	if (body.Follow == nil || *body.Follow) && h.Asks != nil {
		if err := h.Asks.FollowAdded(r.Context(), h.Person(r), id); err != nil {
			log.Printf("api: follow on add: %v", err)
		}
	}
	var t mediafiles.Title
	if err := database.Get(r.Context(), h.Store.Read(), "titles", id, &t); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	v, err := h.TitleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if h.Live != nil {
		h.PublishTitleView(v)
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

// artistID reads an artist Title from the path.
func (h *Handlers) artistID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	t, ok := h.TitleID(w, r)
	if ok && t.Kind != "artist" {
		webserver.Fail(w, http.StatusNotFound, "that Title is not an artist")
		return t, false
	}
	return t, ok
}

const artistHelp = `send any of new_albums (all, new, none), album_types, secondary_types, release_statuses`

func (h *Handlers) setArtist(w http.ResponseWriter, r *http.Request) {
	t, ok := h.artistID(w, r)
	if !ok {
		return
	}
	var body library.ArtistChoices
	if !decode(w, r, &body, artistHelp) {
		return
	}
	if body.NewAlbums == nil && body.AlbumTypes == nil && body.SecondaryTypes == nil && body.ReleaseStatuses == nil {
		webserver.Fail(w, http.StatusBadRequest, artistHelp)
		return
	}
	t, jobID, err := h.lib().SetArtistChoices(r.Context(), t.ID, body, actor(r))
	if err != nil {
		h.musicError(w, err)
		return
	}
	v, err := h.TitleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"title": v, "job_id": jobID})
}

var listOr = ListOr

// albumID reads an album from the path.
func (h *Handlers) albumID(w http.ResponseWriter, r *http.Request) (mediafiles.Part, bool) {
	return AlbumID(w, r, h.Store.Read())
}
