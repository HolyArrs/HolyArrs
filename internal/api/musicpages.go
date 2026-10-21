package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// The artist and album pages: any MusicBrainz artist or album, in the
// Library or not, with what the Library has of it.

// PageAlbum is one album on the artist page.
type PageAlbum struct {
	MBID           string   `json:"mbid"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	SecondaryTypes []string `json:"secondary_types"`
	ReleaseDate    string   `json:"release_date,omitempty"`
	Disambiguation string   `json:"disambiguation,omitempty"`
	// Shelf: albums, singles (EPs and singles), live (live and compilations), other.
	Shelf    string `json:"shelf"`
	Image    string `json:"image"`
	Explicit bool   `json:"explicit,omitempty"`
	Upcoming bool   `json:"upcoming,omitempty"`
	// State is the Library's: complete, partial, missing, downloading,
	// not_out_yet, not_included, or "" when the artist is not in the Library.
	State     string `json:"state,omitempty"`
	PartID    int64  `json:"part_id,omitempty"`
	Following bool   `json:"following,omitempty"`
}

// PageSong is one popular song.
type PageSong struct {
	Title     string `json:"title"`
	LengthMS  int    `json:"length_ms"`
	Explicit  bool   `json:"explicit,omitempty"`
	Album     string `json:"album"`
	AlbumMBID string `json:"album_mbid,omitempty"`
	Image     string `json:"image,omitempty"`
}

// PageArtist is a round artist link.
type PageArtist struct {
	MBID  string `json:"mbid"`
	Name  string `json:"name"`
	Image string `json:"image"`
	Note  string `json:"note,omitempty"`
}

// ArtistPageView answers GET /api/v1/music/artists/{mbid}.
type ArtistPageView struct {
	MBID           string                `json:"mbid"`
	Name           string                `json:"name"`
	Type           string                `json:"type,omitempty"`
	Country        string                `json:"country,omitempty"`
	Disambiguation string                `json:"disambiguation,omitempty"`
	Status         string                `json:"status,omitempty"`
	Genres         []string              `json:"genres"`
	Image          string                `json:"image"`
	TitleID        int64                 `json:"title_id,omitempty"`
	Have           int                   `json:"have"`
	Total          int                   `json:"total"`
	Bio            *metadata.WikiSummary `json:"bio"`
	Albums         []PageAlbum           `json:"albums"`
	Latest         *PageAlbum            `json:"latest"`
	Top            []PageSong            `json:"top"`
	Similar        []PageArtist          `json:"similar"`
	Related        []PageArtist          `json:"related"`
	Kid            bool                  `json:"kid"`
}

// PageTrack is one track on the album page.
type PageTrack struct {
	Disc     int    `json:"disc"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	LengthMS int    `json:"length_ms,omitempty"`
	Explicit bool   `json:"explicit,omitempty"`
	// Have: in_library, downloading, missing, or "" when the album is not kept.
	Have string `json:"have,omitempty"`
}

// PageEdition is one edition (a Pressing) of the album.
type PageEdition struct {
	MBID   string `json:"mbid"`
	Label  string `json:"label"`
	Tracks int    `json:"tracks"`
	Date   string `json:"date,omitempty"`
}

// AlbumPageView answers GET /api/v1/music/albums/{mbid}?artist=<mbid>.
type AlbumPageView struct {
	PageAlbum
	Artist     PageArtist    `json:"artist"`
	TitleID    int64         `json:"title_id,omitempty"`
	Label      string        `json:"label,omitempty"`
	LengthMS   int           `json:"length_ms"`
	Tracks     []PageTrack   `json:"tracks"`
	Editions   []PageEdition `json:"editions"`
	MoreBy     []PageAlbum   `json:"more_by"`
	Similar    []PageArtist  `json:"similar"`
	Kid        bool          `json:"kid"`
	Incomplete bool          `json:"incomplete,omitempty"`
}

type pageCache struct {
	mu   sync.Mutex
	bios map[string]pageBio
	dz   map[string]pageDeezer
}

type pageBio struct {
	v  *metadata.WikiSummary
	at time.Time
}

type pageDeezer struct {
	v  metadata.DeezerProfile
	at time.Time
}

var musicPages = &pageCache{bios: map[string]pageBio{}, dz: map[string]pageDeezer{}}

func (a *API) musicPageRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/music/artists/{mbid}", a.artistPage)
	m.HandleFunc("GET /api/v1/music/albums/{mbid}", a.albumPage)
}

func shelfOf(al metadata.Album) string {
	for _, s := range al.SecondaryTypes {
		switch strings.ToLower(s) {
		case "live", "compilation":
			return "live"
		case "soundtrack", "remix", "dj-mix", "mixtape/street":
			return "other"
		}
		if len(s) > 0 {
			return "other"
		}
	}
	switch strings.ToLower(al.PrimaryType) {
	case "album":
		return "albums"
	case "ep", "single":
		return "singles"
	}
	return "other"
}

func artPath(kind, mbid string) string { return "/api/v1/music-trending/art/" + kind + "/" + mbid }

func (a *API) musicPageOpen(w http.ResponseWriter) bool {
	if p, _ := parts.ForTitleKind("artist"); !a.partIsOn(p) {
		fail(w, http.StatusNotFound, "music is turned off")
		return false
	}
	if a.s.Library == nil || a.s.Library.Artists == nil {
		fail(w, http.StatusServiceUnavailable, "no music source is set up")
		return false
	}
	return true
}

func (a *API) deezerProfile(ctx context.Context, name string) metadata.DeezerProfile {
	key := strings.ToLower(name)
	c := musicPages
	c.mu.Lock()
	e, ok := c.dz[key]
	c.mu.Unlock()
	if ok && time.Since(e.at) < 12*time.Hour {
		return e.v
	}
	if a.s.Library == nil || a.s.Library.ArtistImages == nil {
		return metadata.DeezerProfile{}
	}
	p, err := a.s.Library.ArtistImages.Profile(ctx, name)
	if err == nil {
		c.mu.Lock()
		if len(c.dz) > 2000 {
			c.dz = map[string]pageDeezer{}
		}
		c.dz[key] = pageDeezer{v: p, at: time.Now()}
		c.mu.Unlock()
	}
	return p
}

func (a *API) artistBio(ctx context.Context, name string) *metadata.WikiSummary {
	key := strings.ToLower(name)
	c := musicPages
	c.mu.Lock()
	e, ok := c.bios[key]
	c.mu.Unlock()
	if ok && time.Since(e.at) < 24*time.Hour {
		return e.v
	}
	if a.s.Library == nil || a.s.Library.ArtistImages == nil {
		return nil
	}
	b, err := a.s.Library.ArtistImages.ArtistBio(ctx, name)
	if err == nil && ctx.Err() == nil {
		c.mu.Lock()
		if len(c.bios) > 2000 {
			c.bios = map[string]pageBio{}
		}
		c.bios[key] = pageBio{v: b, at: time.Now()}
		c.mu.Unlock()
	}
	return b
}

// libraryArtist finds the Library's artist with this MusicBrainz id.
func (a *API) libraryArtist(ctx context.Context, mbid string) (*domain.Title, error) {
	ts, err := store.Select[domain.Title](ctx, a.s.Store.Read(), "titles",
		"kind = 'artist' AND state != 'removed' AND lower(json_extract(source_ids, '$.musicbrainz')) = ? LIMIT 1", mbid)
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	return &ts[0], nil
}

// pageLibraryAlbums maps album MBIDs to the Library's state and part id.
func (a *API) pageLibraryAlbums(ctx context.Context, titleID int64) (map[string]library.AlbumSummary, map[int64]bool, error) {
	db := a.s.Store.Read()
	als, err := library.Albums(ctx, db, titleID)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]library.AlbumSummary{}
	for _, al := range als {
		out[strings.ToLower(al.MBID)] = al
	}
	busy := map[int64]bool{}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT i.part_id FROM items i JOIN slots s ON s.item_id = i.id
		WHERE i.title_id = ? AND i.kind = 'track' AND s.state IN ('chosen', 'downloading', 'importing', 'searching')`, titleID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			busy[id] = true
		}
	}
	return out, busy, rows.Err()
}

func pageState(al library.AlbumSummary, busy bool) string {
	if busy {
		return "downloading"
	}
	switch al.State {
	case "complete":
		return "complete"
	case "partial", "missing", "not_out_yet", "not_included":
		return al.State
	}
	return "not_included"
}

func (a *API) albumFollows(ctx context.Context, user int64) map[string]bool {
	out := map[string]bool{}
	rows, err := a.s.Store.Read().QueryContext(ctx, "SELECT mbid FROM album_follows WHERE user_id = ?", user)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out[strings.ToLower(id)] = true
		}
	}
	return out
}

// pageAlbums builds the artist's album list (newest first) with Library
// states, explicit marks and follows.
func (a *API) pageAlbums(ctx context.Context, r *http.Request, art metadata.Artist, dz metadata.DeezerProfile) ([]PageAlbum, *domain.Title, error) {
	t, err := a.libraryArtist(ctx, strings.ToLower(art.MBID))
	if err != nil {
		return nil, nil, err
	}
	var have map[string]library.AlbumSummary
	var busy map[int64]bool
	if t != nil {
		if have, busy, err = a.pageLibraryAlbums(ctx, t.ID); err != nil {
			return nil, nil, err
		}
	}
	explicit := map[string]bool{}
	for _, d := range dz.Albums {
		if d.Explicit {
			explicit[metadata.SongKey(d.Title)] = true
		}
	}
	follows := a.albumFollows(ctx, a.person(r).UserID)
	today := time.Now().UTC().Format("2006-01-02")
	names := []nameKey{{kind: "artist", mbid: strings.ToLower(art.MBID), name: art.Name}}
	out := make([]PageAlbum, 0, len(art.Albums))
	for _, al := range art.Albums {
		id := strings.ToLower(al.MBID)
		p := PageAlbum{MBID: id, Name: al.Name, Type: al.PrimaryType, SecondaryTypes: listOr(al.SecondaryTypes), ReleaseDate: al.ReleaseDate,
			Disambiguation: al.Disambiguation, Shelf: shelfOf(al), Image: artPath("album", id), Explicit: explicit[metadata.SongKey(al.Name)],
			Upcoming: al.ReleaseDate != "" && len(al.ReleaseDate) == 10 && al.ReleaseDate > today, Following: follows[id]}
		if s, ok := have[id]; ok {
			p.State, p.PartID = pageState(s, busy[s.ID]), s.ID
		} else if t != nil {
			p.State = "not_included"
		}
		names = append(names, nameKey{kind: "album", mbid: id, name: al.Name, artist: art.Name})
		out = append(out, p)
	}
	a.rememberMusicNames(names)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ReleaseDate > out[j].ReleaseDate })
	return out, t, nil
}

func (a *API) similarArtists(ctx context.Context, mbid string, n int) []PageArtist {
	out := []PageArtist{}
	if a.s.MusicSimilar == nil {
		return out
	}
	list := a.similarTo([]string{mbid}, 3*time.Second)[mbid]
	var names []nameKey
	for _, s := range list {
		if len(out) >= n || !metadata.ValidMBID(s.MBID) {
			continue
		}
		id := strings.ToLower(s.MBID)
		names = append(names, nameKey{kind: "artist", mbid: id, name: s.Name})
		out = append(out, PageArtist{MBID: id, Name: s.Name, Image: artPath("artist", id)})
	}
	a.rememberMusicNames(names)
	return out
}

func (a *API) artistPage(w http.ResponseWriter, r *http.Request) {
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	if !metadata.ValidMBID(mbid) {
		fail(w, http.StatusBadRequest, "mbid must be a MusicBrainz artist id")
		return
	}
	if !a.musicPageOpen(w) {
		return
	}
	kid := a.kidMax(r) != ""
	ctx := metadata.MemberLookup(r.Context())
	// Genres, links and similar artists need only the id: ask them while
	// the artist itself is looked up.
	var genres []string
	var links []metadata.ArtistLink
	var similar []PageArtist
	var wg sync.WaitGroup
	side, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	wg.Add(2)
	go func() {
		defer wg.Done()
		if src, ok := a.genreProvider(); ok {
			if gl, ok := src.(genreLinkSource); ok {
				genres, links, _ = gl.ArtistGenresLinks(side, mbid)
				return
			}
			genres, _ = src.ArtistGenres(side, mbid)
		}
	}()
	go func() { defer wg.Done(); similar = a.similarArtists(ctx, mbid, 18) }()
	art, err := a.s.Library.Artists.Artist(metadata.QuickArtist(ctx, 1500*time.Millisecond), mbid)
	if errors.Is(err, metadata.ErrArtistNotFound) {
		fail(w, http.StatusNotFound, "no artist with that id")
		return
	}
	if err != nil {
		fail(w, http.StatusBadGateway, "the music source did not answer; try again shortly")
		return
	}
	var dz metadata.DeezerProfile
	var bio *metadata.WikiSummary
	wg.Add(2)
	go func() { defer wg.Done(); dz = a.deezerProfile(side, art.Name) }()
	go func() { defer wg.Done(); bio = a.artistBio(side, art.Name) }()
	wg.Wait()
	albums, t, err := a.pageAlbums(ctx, r, art, dz)
	if err != nil {
		internal(w, err)
		return
	}
	v := ArtistPageView{MBID: mbid, Name: art.Name, Type: art.Type, Country: art.Country, Disambiguation: art.Disambiguation, Status: art.Status,
		Genres: listOr(genres), Image: artPath("artist", mbid), Bio: bio, Similar: similar, Related: []PageArtist{}, Top: []PageSong{}, Kid: kid}
	if t != nil {
		v.TitleID = t.ID
		if img := artistImage(*t); img != "" {
			v.Image = img
		}
	}
	byName := map[string]PageAlbum{}
	kept := albums[:0]
	for _, al := range albums {
		if kid && al.Explicit {
			continue
		}
		kept = append(kept, al)
		if al.Shelf == "albums" || al.Shelf == "singles" {
			v.Total++
			if al.State == "complete" || al.State == "partial" {
				v.Have++
			}
		}
		if k := metadata.SongKey(al.Name); byName[k].MBID == "" || al.Shelf == "albums" {
			byName[k] = al
		}
		if v.Latest == nil && !al.Upcoming && al.ReleaseDate != "" && (al.Shelf == "albums" || al.Shelf == "singles") {
			x := al
			v.Latest = &x
		}
	}
	v.Albums = kept
	for _, s := range dz.Top {
		if kid && s.Explicit {
			continue
		}
		song := PageSong{Title: s.Title, LengthMS: s.LengthMS, Explicit: s.Explicit, Album: s.Album, Image: s.Cover}
		if al, ok := byName[metadata.SongKey(s.Album)]; ok {
			song.AlbumMBID, song.Image = al.MBID, al.Image
		}
		v.Top = append(v.Top, song)
	}
	var names []nameKey
	for _, l := range links {
		names = append(names, nameKey{kind: "artist", mbid: l.MBID, name: l.Name})
		v.Related = append(v.Related, PageArtist{MBID: l.MBID, Name: l.Name, Image: artPath("artist", l.MBID), Note: l.Type})
	}
	a.rememberMusicNames(names)
	a.warmMusicArt(v)
	w.Header().Set("Cache-Control", "private, max-age=60")
	writeJSON(w, http.StatusOK, v)
}

// warmMusicArt starts fetching the page's pictures at once and several at a
// time, so they are kept before the browser asks for each (it asks six at a
// time, and each cold picture is a few outside calls).
func (a *API) warmMusicArt(v ArtistPageView) {
	const pre = "/api/v1/music-trending/art/"
	var keys []string
	add := func(img string) {
		if rest, ok := strings.CutPrefix(img, pre); ok && len(keys) < 80 {
			keys = append(keys, rest)
		}
	}
	add(v.Image)
	for _, al := range v.Albums {
		add(al.Image)
	}
	for _, p := range v.Related {
		add(p.Image)
	}
	for _, p := range v.Similar {
		add(p.Image)
	}
	if len(keys) == 0 || a.s.Library == nil || a.s.Library.Artwork == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		sem := make(chan struct{}, 8)
		var wg sync.WaitGroup
		for _, k := range keys {
			kind, mbid, _ := strings.Cut(k, "/")
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				a.musicTrendArtFile(ctx, kind, mbid)
			}()
		}
		wg.Wait()
	}()
}

func (a *API) albumPage(w http.ResponseWriter, r *http.Request) {
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if !metadata.ValidMBID(mbid) || !metadata.ValidMBID(artist) {
		fail(w, http.StatusBadRequest, "send the album's and its artist's MusicBrainz ids (?artist=)")
		return
	}
	if !a.musicPageOpen(w) {
		return
	}
	kid := a.kidMax(r) != ""
	ctx := metadata.MemberLookup(r.Context())
	// The pressings need only the album id: ask them while the artist is looked up.
	var ps []metadata.Pressing
	var psErr error
	psDone := make(chan struct{})
	go func() { defer close(psDone); ps, psErr = a.s.Library.Artists.Pressings(ctx, mbid) }()
	defer func() { <-psDone }()
	var similar []PageArtist
	simDone := make(chan struct{})
	go func() { defer close(simDone); similar = a.similarArtists(ctx, artist, 14) }()
	defer func() { <-simDone }()
	art, err := a.s.Library.Artists.Artist(metadata.QuickArtist(ctx, 1500*time.Millisecond), artist)
	if err == nil && art.Partial && !hasAlbum(art, mbid) {
		art, err = a.s.Library.Artists.Artist(ctx, artist)
	}
	if err != nil {
		if errors.Is(err, metadata.ErrArtistNotFound) {
			fail(w, http.StatusNotFound, "no artist with that id")
			return
		}
		fail(w, http.StatusBadGateway, "the music source did not answer; try again shortly")
		return
	}
	dz := a.deezerProfile(ctx, art.Name)
	albums, t, err := a.pageAlbums(ctx, r, art, dz)
	if err != nil {
		internal(w, err)
		return
	}
	var me *PageAlbum
	for i := range albums {
		if albums[i].MBID == mbid {
			me = &albums[i]
		}
	}
	if me == nil {
		fail(w, http.StatusNotFound, "that album is not by that artist")
		return
	}
	if kid && me.Explicit {
		fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	v := AlbumPageView{PageAlbum: *me, Artist: PageArtist{MBID: artist, Name: art.Name, Image: artPath("artist", artist)}, Tracks: []PageTrack{},
		Editions: []PageEdition{}, MoreBy: []PageAlbum{}, Kid: kid}
	if t != nil {
		v.TitleID = t.ID
		if img := artistImage(*t); img != "" {
			v.Artist.Image = img
		}
	}
	<-psDone
	if psErr != nil && len(ps) == 0 {
		v.Incomplete = true
	}
	main := mainPressing(ps)
	// Deezer's album: label and explicit marks per track.
	var dzInfo metadata.DeezerAlbumInfo
	for _, d := range dz.Albums {
		if metadata.SongKey(d.Title) == metadata.SongKey(me.Name) {
			dzInfo, _ = a.s.Library.ArtistImages.AlbumInfo(ctx, d.ID)
			break
		}
	}
	v.Label = dzInfo.Label
	have := map[string]string{}
	if me.PartID != 0 {
		have = a.trackStates(ctx, me.PartID)
	}
	if main != nil {
		for _, tr := range main.Tracks {
			pt := PageTrack{Disc: tr.Disc, Number: tr.Number, Name: tr.Name, LengthMS: tr.LengthMS, Explicit: dzInfo.Tracks[metadata.SongKey(tr.Name)]}
			if me.PartID != 0 {
				pt.Have = "missing"
				if s, ok := have[strings.ToLower(tr.RecordingID)]; ok {
					pt.Have = s
				} else if s, ok := have["n:"+metadata.SongKey(tr.Name)]; ok {
					pt.Have = s
				}
			}
			v.LengthMS += tr.LengthMS
			v.Tracks = append(v.Tracks, pt)
		}
		if dzInfo.Explicit {
			v.Explicit = true
		}
	}
	seen := map[string]bool{}
	for _, p := range ps {
		l := editionLabel(p)
		if seen[l] || p.Status != "" && p.Status != "official" {
			continue
		}
		seen[l] = true
		v.Editions = append(v.Editions, PageEdition{MBID: p.MBID, Label: l, Tracks: len(p.Tracks), Date: p.Date})
	}
	for _, al := range albums {
		if al.MBID != mbid && (al.Shelf == "albums" || al.Shelf == "singles") && !(kid && al.Explicit) && len(v.MoreBy) < 20 {
			v.MoreBy = append(v.MoreBy, al)
		}
	}
	<-simDone
	v.Similar = similar
	w.Header().Set("Cache-Control", "private, max-age=60")
	writeJSON(w, http.StatusOK, v)
}

// mainPressing picks the edition to list: the earliest official one with
// the most usual track count.
func hasAlbum(art metadata.Artist, mbid string) bool {
	for _, al := range art.Albums {
		if strings.EqualFold(al.MBID, mbid) {
			return true
		}
	}
	return false
}

func mainPressing(ps []metadata.Pressing) *metadata.Pressing {
	count := map[int]int{}
	for _, p := range ps {
		if len(p.Tracks) > 0 {
			count[len(p.Tracks)]++
		}
	}
	var best *metadata.Pressing
	score := func(p metadata.Pressing) (int, int, string) {
		off := 0
		if p.Status == "official" || p.Status == "" {
			off = 2
		}
		if p.Disambiguation == "" {
			off++ // the plain edition before a deluxe or remaster
		}
		d := p.Date
		if d == "" {
			d = "9999"
		}
		return off, count[len(p.Tracks)], d
	}
	for i := range ps {
		if len(ps[i].Tracks) == 0 {
			continue
		}
		if best == nil {
			best = &ps[i]
			continue
		}
		ao, ac, ad := score(ps[i])
		bo, bc, bd := score(*best)
		if ao > bo || ao == bo && (ac > bc || ac == bc && ad < bd) {
			best = &ps[i]
		}
	}
	return best
}

// trackStates reads the main Version's state per track of one album, by
// recording id and by "n:"+name.
func (a *API) trackStates(ctx context.Context, partID int64) map[string]string {
	out := map[string]string{}
	rows, err := a.s.Store.Read().QueryContext(ctx, `SELECT COALESCE(i.recording_id, ''), COALESCE(i.name, ''), COALESCE(s.state, '') FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = i.title_id ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.part_id = ? AND i.kind = 'track'`, partID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rec, name, st string
		if rows.Scan(&rec, &name, &st) != nil {
			continue
		}
		h := "missing"
		switch domain.SlotState(st) {
		case domain.SlotInLibrary, domain.SlotBetterWanted:
			h = "in_library"
		case domain.SlotChosen, domain.SlotDownloading, domain.SlotImporting, domain.SlotSearching:
			h = "downloading"
		}
		if rec != "" {
			out[strings.ToLower(rec)] = h
		}
		out["n:"+metadata.SongKey(name)] = h
	}
	return out
}

func editionLabel(p metadata.Pressing) string {
	l := p.Name
	if p.Disambiguation != "" {
		l += " (" + p.Disambiguation + ")"
	}
	return l
}
