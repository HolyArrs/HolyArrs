package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// musicRoutes serves adding artists, their albums, Pressings and tracks,
// and the music settings (stage 2.3).
func (a *API) musicRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/artists/lookup", a.lookupArtists)
	m.HandleFunc("POST /api/v1/artists", a.addArtist)
	m.HandleFunc("PUT /api/v1/titles/{id}/artist", a.setArtist)
	m.HandleFunc("GET /api/v1/albums/{id}/tracks", a.listTracks)
	m.HandleFunc("GET /api/v1/albums/{id}/cover", a.albumCoverArt)
}

// musicError answers music refusals with a status and a reason; anything
// else goes to libraryError.
func musicError(w http.ResponseWriter, err error) { music.Error(w, err, libraryError) }

// libraryHandlers serves albums and the music settings.
func (a *API) libraryHandlers() music.LibraryHandlers {
	return music.LibraryHandlers{Store: a.s.Store, Library: a.s.Library, ArtistID: a.artistID, Actor: actor, Fallback: libraryError}
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
	Ask  *askBrief  `json:"ask,omitempty"`
	Asks []askBrief `json:"asks,omitempty"`
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
func (a *API) askableAlbums(ctx context.Context, titleID int64, albums []metadata.Album) ([]AskableAlbum, error) {
	byMBID := map[string]mediafiles.Part{}
	artistKeep := false
	if titleID != 0 {
		db := a.s.Store.Read()
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
func (a *API) lookupArtists(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if term == "" || len(term) > 200 {
		webserver.Fail(w, http.StatusBadRequest, "send ?term=<artist name> or an id such as mbid:5b11f4ce-a62d-471e-81fc-a69a8278c7da")
		return
	}
	if a.kidMax(r) != "" {
		// Music has no age ratings, so a Kid finds none.
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"artists": []ArtistResult{}, "source": "musicbrainz"})
		return
	}
	me := a.person(r).UserID
	if !a.lookups.take(me, maxLookupsEach) {
		w.Header().Set("Retry-After", "5")
		webserver.Fail(w, http.StatusTooManyRequests, "you already have two music lookups running; wait for them to finish and try again")
		return
	}
	defer a.lookups.give(me)
	ctx := r.Context()
	if !mayApprove(r) {
		// Members wait behind the scheduled refresh and staff at MusicBrainz.
		ctx = metadata.MemberLookup(ctx)
	}
	found, err := a.s.Library.LookupArtists(ctx, term)
	if errors.Is(err, library.ErrSearchFilling) {
		// An unseen name: the search keeps running behind and is kept, so
		// the next ask is answered from the store.
		w.Header().Set("Retry-After", "2")
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"artists": []ArtistResult{}, "source": "musicbrainz", "pending": true})
		return
	}
	if err != nil {
		musicError(w, err)
		return
	}
	out := []ArtistResult{}
	db := a.s.Store.Read()
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
		if v.Asks, err = a.openArtistAsks(r.Context(), ar.MBID, me); err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for i := range v.Asks {
			if v.Ask == nil || (v.Asks[i].Mine && !v.Ask.Mine) {
				v.Ask = &v.Asks[i]
			}
		}
		if withAlbums && albums != nil {
			if v.AskAlbums, err = a.askableAlbums(r.Context(), v.TitleID, albums); err != nil {
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
	a.rememberMusicNames(names)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"artists": out, "source": "musicbrainz"})
}

const addArtistHelp = `send {"id": "mbid:<MusicBrainz artist id>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, albums (all, future, missing, latest, first, none), new_albums (all, new, none), album_types (album, ep, single, broadcast, other), secondary_types (studio, live, compilation, ...), release_statuses (official, promotion, bootleg, pseudo-release), follow`

func (a *API) addArtist(w http.ResponseWriter, r *http.Request) {
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
	id, jobID, err := a.s.Library.AddArtist(r.Context(), body.ID, body.ArtistAddOptions, actor(r))
	if err != nil {
		musicError(w, err)
		return
	}
	if (body.Follow == nil || *body.Follow) && a.s.Asks != nil {
		if err := a.s.Asks.FollowAdded(r.Context(), a.person(r), id); err != nil {
			log.Printf("api: follow on add: %v", err)
		}
	}
	var t mediafiles.Title
	if err := database.Get(r.Context(), a.s.Store.Read(), "titles", id, &t); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if a.s.Live != nil {
		a.publishTitleView(v)
	}
	webserver.WriteJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

// artistID reads an artist Title from the path.
func (a *API) artistID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	t, ok := a.titleID(w, r)
	if ok && t.Kind != "artist" {
		webserver.Fail(w, http.StatusNotFound, "that Title is not an artist")
		return t, false
	}
	return t, ok
}

const artistHelp = `send any of new_albums (all, new, none), album_types, secondary_types, release_statuses`

func (a *API) setArtist(w http.ResponseWriter, r *http.Request) {
	t, ok := a.artistID(w, r)
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
	t, jobID, err := a.s.Library.SetArtistChoices(r.Context(), t.ID, body, actor(r))
	if err != nil {
		musicError(w, err)
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"title": v, "job_id": jobID})
}

// ArtistInfo is what an artist's page needs besides the shared Title fields.
type ArtistInfo struct {
	MBID            string     `json:"mbid"`
	Type            string     `json:"type,omitempty"`
	Disambiguation  string     `json:"disambiguation,omitempty"`
	Status          string     `json:"status"` // running, ended
	AlbumChoice     string     `json:"album_choice,omitempty"`
	NewAlbums       string     `json:"new_albums"`
	AlbumTypes      []string   `json:"album_types"`
	SecondaryTypes  []string   `json:"secondary_types"`
	ReleaseStatuses []string   `json:"release_statuses"`
	Albums          int        `json:"albums"`   // every album row
	Included        int        `json:"included"` // albums the kinds include
	Tracks          int        `json:"tracks"`
	InLibrary       int        `json:"in_library"`
	Missing         int        `json:"missing"`
	NotOutYet       int        `json:"not_out_yet"`
	BetterWanted    int        `json:"better_wanted"`
	Downloading     int        `json:"downloading"`
	Failed          int        `json:"failed"`
	NextRelease     *time.Time `json:"next_release,omitempty"`
	// Bio is the start of the artist's kept Wikipedia summary (Library list
	// only, read from what is already stored; empty when none is kept).
	Bio string `json:"bio,omitempty"`
}

var listOr = music.ListOr

// PressingView is one Pressing of an album with its tracks.
type PressingView = music.PressingView

func (a *API) artistInfo(ctx context.Context, t mediafiles.Title) (*ArtistInfo, error) {
	n, err := library.Totals(ctx, a.s.Store.Read(), t.ID, time.Now())
	if err != nil {
		return nil, err
	}
	return &ArtistInfo{MBID: t.SourceIDs["musicbrainz"], Type: strOr(t.ArtistType), Disambiguation: strOr(t.Disambiguation), Status: t.Status,
		AlbumChoice: strOr(t.AlbumChoice), NewAlbums: library.NewAlbumsOf(t.NewPartsPolicy), AlbumTypes: listOr(t.MusicTypes),
		SecondaryTypes: listOr(t.MusicSecondary), ReleaseStatuses: listOr(t.MusicStatuses), Albums: n.Albums, Included: n.Included,
		Tracks: n.Tracks, InLibrary: n.InLibrary, Missing: n.Missing, NotOutYet: n.NotOutYet, BetterWanted: n.BetterWanted,
		Downloading: n.Downloading, Failed: n.Failed, NextRelease: n.NextRelease}, nil
}

// albumID reads an album from the path.
func (a *API) albumID(w http.ResponseWriter, r *http.Request) (mediafiles.Part, bool) {
	return music.AlbumID(w, r, a.s.Store.Read())
}

// TrackView is one track with its Slots.
type TrackView struct {
	ID           int64             `json:"id"`
	Disc         int               `json:"disc"`
	Number       int               `json:"number"`
	Name         string            `json:"name,omitempty"`
	LengthMS     *int              `json:"length_ms,omitempty"`
	RecordingID  string            `json:"recording_id,omitempty"`
	ReleaseDate  *time.Time        `json:"release_date,omitempty"`
	KeepUpToDate bool              `json:"keep_up_to_date"`
	Dropped      bool              `json:"dropped,omitempty"`
	IDs          map[string]string `json:"ids"`
	Slots        []EpisodeSlot     `json:"slots"`
}

// listTracks lists an album's tracks in disc and track order, each with its
// Slot per Version.
func (a *API) listTracks(w http.ResponseWriter, r *http.Request) {
	p, ok := a.albumID(w, r)
	if !ok {
		return
	}
	db := a.s.Store.Read()
	its, err := database.Select[mediafiles.Item](r.Context(), db, "items", `part_id = ? AND kind = 'track'
		ORDER BY json_extract(numbers, '$.disc'), json_extract(numbers, '$.track'), id`, p.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	vs, err := database.Select[mediafiles.Version](r.Context(), db, "versions", "title_id = ? ORDER BY is_main DESC, id", p.TitleID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	label := map[int64]string{}
	for _, v := range vs {
		label[v.ID] = v.Label
	}
	slots, err := database.Select[mediafiles.Slot](r.Context(), db, "slots", "item_id IN (SELECT id FROM items WHERE part_id = ? AND kind = 'track') ORDER BY id", p.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	byItem := map[int64][]EpisodeSlot{}
	for _, s := range slots {
		es := EpisodeSlot{ID: s.ID, Version: label[s.VersionID], VersionID: s.VersionID, State: string(s.State), StateLabel: StateLabels[s.State], FileID: s.FileID}
		if s.LastReason != nil {
			es.Reason = *s.LastReason
		}
		byItem[s.ItemID] = append(byItem[s.ItemID], es)
	}
	trimEpisodeSlots(r.Context(), byItem)
	out := make([]TrackView, 0, len(its))
	for _, it := range its {
		v := TrackView{ID: it.ID, Name: strOr(it.Name), LengthMS: it.LengthMS, RecordingID: strOr(it.RecordingID), ReleaseDate: it.AirDate,
			KeepUpToDate: it.KeepUpToDate, Dropped: it.DroppedAt != nil, IDs: it.SourceIDs, Slots: []EpisodeSlot{}}
		if it.Numbers.Disc != nil {
			v.Disc = *it.Numbers.Disc
		}
		if it.Numbers.Track != nil {
			v.Number = *it.Numbers.Track
		}
		if v.IDs == nil {
			v.IDs = map[string]string{}
		}
		if sl := byItem[it.ID]; sl != nil {
			v.Slots = sl
		}
		out = append(out, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tracks": out, "album_id": p.ID})
}
