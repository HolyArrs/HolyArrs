package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// musicRoutes serves adding artists, their albums, Pressings and tracks,
// and the music settings (stage 2.3).
func (a *API) musicRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/artists/lookup", a.lookupArtists)
	m.HandleFunc("POST /api/v1/artists", a.addArtist)
	m.HandleFunc("PUT /api/v1/titles/{id}/artist", a.setArtist)
	m.HandleFunc("GET /api/v1/titles/{id}/albums", a.listAlbums)
	m.HandleFunc("GET /api/v1/albums/{id}", a.getAlbum)
	m.HandleFunc("PUT /api/v1/albums/{id}", a.setAlbum)
	m.HandleFunc("GET /api/v1/albums/{id}/tracks", a.listTracks)
	m.HandleFunc("GET /api/v1/settings/music", a.getMusicSettings)
	m.HandleFunc("PUT /api/v1/settings/music", a.putMusicSettings)
}

// musicError answers music refusals with a status and a reason; anything
// else goes to libraryError.
func musicError(w http.ResponseWriter, err error) {
	var dup *library.ArtistAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, library.ErrNoMusicFolder), errors.Is(err, library.ErrArtistNeverAdd), errors.Is(err, library.ErrAlbumNotIncluded):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadAlbumChoice), errors.Is(err, library.ErrBadNewAlbums), errors.Is(err, library.ErrBadAlbumKinds),
		errors.Is(err, library.ErrBadExtraKinds), errors.Is(err, library.ErrBadStatuses), errors.Is(err, library.ErrBadPressingRule),
		errors.Is(err, library.ErrBadMusicURL), errors.Is(err, library.ErrBadCacheHours), errors.Is(err, library.ErrNotMusicFolder),
		errors.Is(err, library.ErrBadArtistID):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrNotArtist), errors.Is(err, library.ErrNotAlbum), errors.Is(err, library.ErrNoSuchPressing),
		errors.Is(err, metadata.ErrArtistNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, library.ErrNoMusicSource), errors.Is(err, metadata.ErrMusicBusy):
		fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, metadata.ErrMusicUnavailable):
		// Plain words only: no status code or connection detail.
		fail(w, http.StatusBadGateway, metadata.ErrMusicUnavailable.Error())
	default:
		libraryError(w, err)
	}
}

// ArtistResult is a looked-up artist (without its albums) with what the
// Library knows of it.
type ArtistResult struct {
	metadata.Artist
	TitleID  int64 `json:"title_id,omitempty"` // set when already in the Library
	NeverAdd bool  `json:"never_add,omitempty"`
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
	byMBID := map[string]domain.Part{}
	artistKeep := false
	if titleID != 0 {
		db := a.s.Store.Read()
		if err := db.QueryRowContext(ctx, "SELECT keep_up_to_date FROM titles WHERE id = ?", titleID).Scan(&artistKeep); err != nil {
			return nil, err
		}
		ps, err := store.Select[domain.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL", titleID)
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
		fail(w, http.StatusBadRequest, "send ?term=<artist name> or an id such as mbid:5b11f4ce-a62d-471e-81fc-a69a8278c7da")
		return
	}
	me := a.person(r).UserID
	if !a.lookups.take(me, maxLookupsEach) {
		w.Header().Set("Retry-After", "5")
		fail(w, http.StatusTooManyRequests, "you already have two music lookups running; wait for them to finish and try again")
		return
	}
	defer a.lookups.give(me)
	ctx := r.Context()
	if !mayApprove(r) {
		// Members wait behind the scheduled refresh and staff at MusicBrainz.
		ctx = metadata.MemberLookup(ctx)
	}
	found, err := a.s.Library.LookupArtists(ctx, term)
	if err != nil {
		musicError(w, err)
		return
	}
	out := []ArtistResult{}
	db := a.s.Store.Read()
	withAlbums := r.URL.Query().Get("albums") == "true"
	for _, ar := range found {
		albums := ar.Albums
		ar.Albums = nil
		v := ArtistResult{Artist: ar}
		if v.TitleID, err = library.ArtistByMBID(r.Context(), db, ar.MBID); err != nil {
			internal(w, err)
			return
		}
		if v.Asks, err = a.openArtistAsks(r.Context(), ar.MBID, me); err != nil {
			internal(w, err)
			return
		}
		for i := range v.Asks {
			if v.Ask == nil || (v.Asks[i].Mine && !v.Ask.Mine) {
				v.Ask = &v.Asks[i]
			}
		}
		if withAlbums && albums != nil {
			if v.AskAlbums, err = a.askableAlbums(r.Context(), v.TitleID, albums); err != nil {
				internal(w, err)
				return
			}
		}
		if v.NeverAdd, err = library.ArtistOnNeverAdd(r.Context(), db, ar.MBID); err != nil {
			internal(w, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"artists": out, "source": "musicbrainz"})
}

const addArtistHelp = `send {"id": "mbid:<MusicBrainz artist id>"} with optional library_folder_id, pick_rules_id, keep_up_to_date, albums (all, future, missing, latest, first, none), new_albums (all, new, none), album_types (album, ep, single, broadcast, other), secondary_types (studio, live, compilation, ...), release_statuses (official, promotion, bootleg, pseudo-release)`

func (a *API) addArtist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
		library.ArtistAddOptions
	}
	if !decode(w, r, &body, addArtistHelp) {
		return
	}
	if strings.TrimSpace(body.ID) == "" {
		fail(w, http.StatusBadRequest, addArtistHelp)
		return
	}
	id, jobID, err := a.s.Library.AddArtist(r.Context(), body.ID, body.ArtistAddOptions, actor(r))
	if err != nil {
		musicError(w, err)
		return
	}
	var t domain.Title
	if err := store.Get(r.Context(), a.s.Store.Read(), "titles", id, &t); err != nil {
		internal(w, err)
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		internal(w, err)
		return
	}
	if a.s.Live != nil {
		a.s.Live.Publish("title", v)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"title": v, "job_id": jobID})
}

// artistID reads an artist Title from the path.
func (a *API) artistID(w http.ResponseWriter, r *http.Request) (domain.Title, bool) {
	t, ok := a.titleID(w, r)
	if ok && t.Kind != "artist" {
		fail(w, http.StatusNotFound, "that Title is not an artist")
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
		fail(w, http.StatusBadRequest, artistHelp)
		return
	}
	t, jobID, err := a.s.Library.SetArtistChoices(r.Context(), t.ID, body, actor(r))
	if err != nil {
		musicError(w, err)
		return
	}
	v, err := a.titleView(r.Context(), t)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"title": v, "job_id": jobID})
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
	NextRelease     *time.Time `json:"next_release,omitempty"`
}

func listOr(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func (a *API) artistInfo(ctx context.Context, t domain.Title) (*ArtistInfo, error) {
	als, err := library.Albums(ctx, a.s.Store.Read(), t.ID)
	if err != nil {
		return nil, err
	}
	ai := &ArtistInfo{MBID: t.SourceIDs["musicbrainz"], Type: strOr(t.ArtistType), Disambiguation: strOr(t.Disambiguation), Status: t.Status,
		AlbumChoice: strOr(t.AlbumChoice), NewAlbums: library.NewAlbumsOf(t.NewPartsPolicy), AlbumTypes: listOr(t.MusicTypes),
		SecondaryTypes: listOr(t.MusicSecondary), ReleaseStatuses: listOr(t.MusicStatuses), Albums: len(als)}
	now := time.Now()
	for _, al := range als {
		if al.Included {
			ai.Included++
			if al.ReleaseDate != nil && al.ReleaseDate.After(now) && (ai.NextRelease == nil || al.ReleaseDate.Before(*ai.NextRelease)) {
				d := *al.ReleaseDate
				ai.NextRelease = &d
			}
		}
		ai.Tracks += al.Tracks
		ai.InLibrary += al.InLibrary
		ai.Missing += al.Missing
		ai.NotOutYet += al.NotOutYet
		ai.BetterWanted += al.BetterWanted
	}
	return ai, nil
}

// listAlbums lists an artist's albums, newest first; ?included=true leaves
// out the albums the artist's kinds do not include.
func (a *API) listAlbums(w http.ResponseWriter, r *http.Request) {
	t, ok := a.artistID(w, r)
	if !ok {
		return
	}
	als, err := library.Albums(r.Context(), a.s.Store.Read(), t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if r.URL.Query().Get("included") == "true" {
		kept := als[:0]
		for _, al := range als {
			if al.Included {
				kept = append(kept, al)
			}
		}
		als = kept
	}
	writeJSON(w, http.StatusOK, map[string]any{"albums": als})
}

// albumID reads an album from the path.
func (a *API) albumID(w http.ResponseWriter, r *http.Request) (domain.Part, bool) {
	var p domain.Part
	id, ok := pathID(w, r)
	if !ok {
		return p, false
	}
	if err := store.Get(r.Context(), a.s.Store.Read(), "parts", id, &p); err != nil || p.Kind != "album" {
		fail(w, http.StatusNotFound, "no such album")
		return p, false
	}
	return p, true
}

// PressingView is one Pressing of an album with its tracks.
type PressingView struct {
	ID             int64                  `json:"id"`
	MBID           string                 `json:"mbid"`
	Name           string                 `json:"name"`
	Status         string                 `json:"status"`
	Country        string                 `json:"country,omitempty"`
	Date           string                 `json:"date,omitempty"`
	Disambiguation string                 `json:"disambiguation,omitempty"`
	Formats        []string               `json:"formats"`
	TrackCount     int                    `json:"track_count"`
	Chosen         bool                   `json:"chosen"`
	ChosenBy       string                 `json:"chosen_by,omitempty"` // auto, person (the chosen one only)
	Tracks         []domain.PressingTrack `json:"tracks"`
}

func (a *API) getAlbum(w http.ResponseWriter, r *http.Request) {
	p, ok := a.albumID(w, r)
	if !ok {
		return
	}
	sum, err := library.AlbumOf(r.Context(), a.s.Store.Read(), p)
	if err != nil {
		internal(w, err)
		return
	}
	ps, err := store.Select[domain.AlbumPressing](r.Context(), a.s.Store.Read(), "pressings", "part_id = ? ORDER BY chosen DESC, date, id", p.ID)
	if err != nil {
		internal(w, err)
		return
	}
	views := make([]PressingView, 0, len(ps))
	for _, x := range ps {
		v := PressingView{ID: x.ID, MBID: x.MBID, Name: x.Name, Status: x.Status, Country: x.Country, Date: x.Date, Disambiguation: x.Disambiguation,
			Formats: listOr(x.Formats), TrackCount: x.TrackCount, Chosen: x.Chosen, Tracks: x.Tracks}
		if x.Chosen {
			v.ChosenBy = x.ChosenBy
		}
		if v.Tracks == nil {
			v.Tracks = []domain.PressingTrack{}
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"album": sum, "title_id": p.TitleID, "pressings": views})
}

const albumHelp = `send {"keep_up_to_date": true or false} and/or {"pressing_id": <id of one of the album's Pressings>}`

func (a *API) setAlbum(w http.ResponseWriter, r *http.Request) {
	p, ok := a.albumID(w, r)
	if !ok {
		return
	}
	var body struct {
		KeepUpToDate *bool `json:"keep_up_to_date"`
		PressingID   int64 `json:"pressing_id"`
	}
	if !decode(w, r, &body, albumHelp) {
		return
	}
	if body.KeepUpToDate == nil && body.PressingID == 0 {
		fail(w, http.StatusBadRequest, albumHelp)
		return
	}
	p, err := a.s.Library.SetAlbum(r.Context(), p.ID, body.KeepUpToDate, body.PressingID, actor(r))
	if err != nil {
		musicError(w, err)
		return
	}
	sum, err := library.AlbumOf(r.Context(), a.s.Store.Read(), p)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
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
	its, err := store.Select[domain.Item](r.Context(), db, "items", `part_id = ? AND kind = 'track'
		ORDER BY json_extract(numbers, '$.disc'), json_extract(numbers, '$.track'), id`, p.ID)
	if err != nil {
		internal(w, err)
		return
	}
	vs, err := store.Select[domain.Version](r.Context(), db, "versions", "title_id = ? ORDER BY is_main DESC, id", p.TitleID)
	if err != nil {
		internal(w, err)
		return
	}
	label := map[int64]string{}
	for _, v := range vs {
		label[v.ID] = v.Label
	}
	slots, err := store.Select[domain.Slot](r.Context(), db, "slots", "item_id IN (SELECT id FROM items WHERE part_id = ? AND kind = 'track') ORDER BY id", p.ID)
	if err != nil {
		internal(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"tracks": out, "album_id": p.ID})
}

func (a *API) getMusicSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadMusicDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *API) putMusicSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadMusicDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	if !decode(w, r, &d, "send any of album_choice, new_albums, album_types, secondary_types, release_statuses, pressing_rule, musicbrainz_url, musicbrainz_allow_local_network, cache_hours") {
		return
	}
	if err := library.SetMusicDefaults(r.Context(), a.s.Store, d, actor(r)); err != nil {
		musicError(w, err)
		return
	}
	d, err = library.ReadMusicDefaults(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
