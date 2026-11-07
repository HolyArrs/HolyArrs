package api

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
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
	// Shelf: albums, singles (EPs and singles), compilations, live, other.
	Shelf    string `json:"shelf"`
	Image    string `json:"image"`
	Explicit bool   `json:"explicit,omitempty"`
	// RecordingID is the song's MusicBrainz recording (the same on every edition).
	RecordingID string `json:"recording_id,omitempty"`
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

// PageStoreAlbum is one release of the discography fallback; Image goes
// through the picture route like a top song's release.
type PageStoreAlbum struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date,omitempty"`
	Type        string `json:"type"`
	TrackCount  int    `json:"track_count,omitempty"`
	Explicit    bool   `json:"explicit,omitempty"`
	Image       string `json:"image,omitempty"`
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
	// Popularity is the artist's listening figures, nil when none is known.
	Popularity *metadata.ArtistPopularity `json:"popularity,omitempty"`
	// StoreAlbums is the discography a store knows when MusicBrainz has no
	// release groups: shown, never asked for (no MusicBrainz id).
	StoreAlbums []PageStoreAlbum `json:"store_albums,omitempty"`
	Similar     []PageArtist     `json:"similar"`
	Related        []PageArtist          `json:"related"`
	Kid            bool                  `json:"kid"`
	// Incomplete: the music source did not answer in time; the page shows
	// what is known and asks again.
	Incomplete bool `json:"incomplete,omitempty"`
	// Gap: the metadata server is still fetching this artist (or is queued);
	// the page shows its message in place of the content and asks again.
	Gap *PageGap `json:"gap,omitempty"`
	// Missing is what "Ask for all missing" asks: the released albums,
	// EPs and singles the Library does not keep, and about how big.
	Missing PageMissing `json:"missing"`
}

// PageMissing is the confirm line of "Ask for all missing": N releases, about
// SizeBytes, and their ids (the ask carries exactly these).
type PageMissing struct {
	Count     int      `json:"count"`
	SizeBytes int64    `json:"size_bytes"`
	Albums    []string `json:"albums"`
}

// The size guess per release for the confirm line: a lossless album and a
// short release (EP or single). Shown as "about", never used to pick copies.
const (
	missingAlbumBytes  = 450 << 20
	missingSingleBytes = 90 << 20
)

// PageCredit is a release on another artist's page or a neighbour's top
// album: the album and whose it is.
type PageCredit struct {
	PageAlbum
	ArtistMBID string `json:"artist_mbid"`
	ArtistName string `json:"artist_name"`
	Listeners  int64  `json:"listeners,omitempty"`
}

// PageTrack is one track on the album page.
type PageTrack struct {
	Disc     int    `json:"disc"`
	Number   int    `json:"number"`
	Name     string `json:"name"`
	LengthMS int    `json:"length_ms,omitempty"`
	Explicit bool   `json:"explicit,omitempty"`
	// RecordingID is the song's MusicBrainz recording (the same on every edition).
	RecordingID string `json:"recording_id,omitempty"`
	// Have: in_library, downloading, missing, or "" when the album is not kept.
	Have string `json:"have,omitempty"`
}

// PageEdition is one edition (a Pressing) of the album.
type PageEdition struct {
	MBID   string `json:"mbid"`
	Label  string `json:"label"`
	Tracks int    `json:"tracks"`
	Date   string `json:"date,omitempty"`
	// List is this edition's tracklist, with the Library's have per track.
	List []PageTrack `json:"list"`
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
	Gap        *PageGap      `json:"gap,omitempty"`
}

// PageGap is the metadata server's word on an item it is still getting:
// state "fetching" or "queued", the place in line, and the words to show.
type PageGap struct {
	State    string `json:"state"`
	Position int    `json:"position,omitempty"`
	Message  string `json:"message"`
}

// gapOf reads a still-fetching answer out of err; nil when err is not one.
func gapOf(err error) *PageGap {
	var p *metadata.PendingError
	if errors.As(err, &p) {
		return &PageGap{State: p.State, Position: p.Position, Message: p.Message}
	}
	if errors.Is(err, metadata.ErrServicePending) {
		return &PageGap{State: "fetching", Message: "Getting this from the server…"}
	}
	return nil
}

// pageTags are an artist's genres and linked artists (one MusicBrainz call,
// which its limiter spaces a second apart).
type pageTags struct {
	Genres []string              `json:"genres,omitempty"`
	Links  []metadata.ArtistLink `json:"links,omitempty"`
}

// artistTags looks up an artist's genres and links, fresh 12 hours (kept
// on disk, served stale while asked again).
func (a *API) artistTags(ctx context.Context, mbid string) ([]string, []metadata.ArtistLink) {
	src, ok := a.genreProvider()
	if !ok {
		return nil, nil
	}
	t, _ := sideGet(a, ctx, "tags", strings.ToLower(mbid), 12*time.Hour, func(ctx context.Context) (pageTags, bool, error) {
		var t pageTags
		var err error
		if gl, ok := src.(genreLinkSource); ok {
			t.Genres, t.Links, err = gl.ArtistGenresLinks(ctx, mbid)
		} else {
			t.Genres, err = src.ArtistGenres(ctx, mbid)
		}
		return t, true, err
	})
	return t.Genres, t.Links
}

func (a *API) musicPageRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/music/artists/{mbid}", a.artistPage)
	m.HandleFunc("GET /api/v1/music/albums/{mbid}", a.albumPage)
}

func shelfOf(al metadata.Album) string {
	for _, s := range al.SecondaryTypes {
		switch strings.ToLower(s) {
		case "compilation":
			return "compilations"
		case "live":
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

// isVideo is a release group that is a video; the pages leave them out.
func isVideo(al metadata.Album) bool {
	if strings.EqualFold(al.PrimaryType, "video") {
		return true
	}
	for _, s := range al.SecondaryTypes {
		if strings.EqualFold(s, "video") {
			return true
		}
	}
	return false
}

// songAlbum finds a top song's release in the discography by name, also
// without an edition after a dash ("Born To Die – Paradise Edition" is
// "Born To Die").
func songAlbum[T any](byName map[string]T, album string) (T, bool) {
	if al, ok := byName[metadata.SongKey(album)]; ok {
		return al, true
	}
	for _, dash := range []string{" – ", " — ", " - "} {
		if i := strings.Index(album, dash); i > 0 {
			if al, ok := byName[metadata.SongKey(album[:i])]; ok {
				return al, true
			}
		}
	}
	var zero T
	return zero, false
}

func artPath(kind, mbid string) string { return "/api/v1/music-trending/art/" + kind + "/" + mbid }

// songArtID names the release of a top song that has no MusicBrainz id in
// the discography, for the picture route ("song-" and 24 hex digits).
func songArtID(artist, album, cover string) string {
	sum := sha1.Sum([]byte(strings.ToLower(artist) + "|" + strings.ToLower(album) + "|" + cover))
	return "song-" + hex.EncodeToString(sum[:12])
}

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

// artistIDs reads an artist's Deezer and Wikidata ids from its MusicBrainz
// links, fresh a day (misses an hour).
func (a *API) artistIDs(ctx context.Context, mbid string) metadata.ArtistIDs {
	mbid = strings.ToLower(mbid)
	if a.s.Library == nil {
		return metadata.ArtistIDs{}
	}
	src, ok := a.s.Library.Artists.(metadata.ArtistIDSource)
	if !ok {
		return metadata.ArtistIDs{}
	}
	ids, _ := sideGet(a, ctx, "ids", mbid, 24*time.Hour, func(ctx context.Context) (metadata.ArtistIDs, bool, error) {
		ids, err := src.ArtistIDs(ctx, mbid)
		if errors.Is(err, metadata.ErrArtistNotFound) {
			return ids, false, nil
		}
		return ids, err == nil && ids != (metadata.ArtistIDs{}), err
	})
	return ids
}

// deezerProfile is the artist's Deezer profile: by the Deezer id from its
// MusicBrainz links, else by exact name with album titles that agree.
// Fresh 12 hours; no sure match is a miss, asked again after an hour (the
// artist's links may have come by then).
func (a *API) deezerProfile(ctx context.Context, mbid string, art metadata.Artist) metadata.DeezerProfile {
	if a.s.MusicDeezer != nil {
		// The metadata server's kept match (by the artist's own links, else
		// a checked name search there); a miss is asked again after an hour.
		p, _ := sideGet(a, ctx, "deezer2", strings.ToLower(mbid), 12*time.Hour, func(ctx context.Context) (metadata.DeezerProfile, bool, error) {
			p, err := a.s.MusicDeezer(ctx, mbid)
			return p, err == nil && p.ID != 0, err
		})
		return p
	}
	if a.s.Library == nil || a.s.Library.ArtistImages == nil {
		return metadata.DeezerProfile{}
	}
	name := art.Name
	albums := make([]string, 0, len(art.Albums))
	for _, al := range art.Albums {
		albums = append(albums, al.Name)
	}
	p, _ := sideGet(a, ctx, "deezer", strings.ToLower(mbid), 12*time.Hour, func(ctx context.Context) (metadata.DeezerProfile, bool, error) {
		if name == "" {
			return metadata.DeezerProfile{}, false, errors.New("no artist name yet")
		}
		p, err := a.s.Library.ArtistImages.ProfileFor(ctx, name, a.artistIDs(ctx, mbid).Deezer, albums)
		return p, err == nil && p.ID != 0, err
	})
	return p
}

// artistBio is the artist's Wikipedia summary, fresh a day (none: a miss,
// asked again after an hour): the metadata server's (the article the
// artist's Wikidata item links), else the developer stand-in's by name.
func (a *API) artistBio(ctx context.Context, mbid, name string) *metadata.WikiSummary {
	if a.s.MusicArtistBio != nil {
		b, _ := sideGet(a, ctx, "bio2", strings.ToLower(mbid), 24*time.Hour, func(ctx context.Context) (*metadata.WikiSummary, bool, error) {
			b, err := a.s.MusicArtistBio(ctx, mbid)
			return b, err == nil && b != nil, err
		})
		return b
	}
	if a.s.Library == nil || a.s.Library.ArtistImages == nil || strings.TrimSpace(name) == "" {
		return nil
	}
	b, _ := sideGet(a, ctx, "bio", strings.ToLower(name), 24*time.Hour, func(ctx context.Context) (*metadata.WikiSummary, bool, error) {
		b, err := a.s.Library.ArtistImages.ArtistBio(ctx, name)
		return b, err == nil && b != nil, err
	})
	return b
}

// deezerAlbum is one Deezer album's label and per-track explicit marks:
// the metadata server's, kept here 14 days (a miss an hour), else the
// developer stand-in's.
func (a *API) deezerAlbum(ctx context.Context, id int64) metadata.DeezerAlbumInfo {
	if a.s.MusicDeezerAlbum != nil {
		type kept struct {
			Label    string          `json:"label"`
			Explicit bool            `json:"explicit"`
			Tracks   map[string]bool `json:"tracks"`
		}
		k, ok := sideGet(a, ctx, "dzalbum", strconv.FormatInt(id, 10), 14*24*time.Hour, func(ctx context.Context) (kept, bool, error) {
			info, err := a.s.MusicDeezerAlbum(ctx, id)
			return kept{info.Label, info.Explicit, info.Tracks}, err == nil && (info.Label != "" || len(info.Tracks) > 0), err
		})
		if !ok {
			return metadata.DeezerAlbumInfo{Tracks: map[string]bool{}}
		}
		if k.Tracks == nil {
			k.Tracks = map[string]bool{}
		}
		return metadata.DeezerAlbumInfo{Label: k.Label, Explicit: k.Explicit, Tracks: k.Tracks}
	}
	if a.s.Library == nil || a.s.Library.ArtistImages == nil {
		return metadata.DeezerAlbumInfo{Tracks: map[string]bool{}}
	}
	info, _ := a.s.Library.ArtistImages.AlbumInfo(ctx, id)
	return info
}

// artistTopSongs is the metadata server's top songs of the artist, fresh 6
// hours; nil when it has none or did not answer (the Deezer list stands in).
func (a *API) artistTopSongs(ctx context.Context, mbid string) *metadata.MusicTopSongs {
	if a.s.MusicTopSongs == nil {
		return nil
	}
	t, ok := sideGet(a, ctx, "top", strings.ToLower(mbid), 6*time.Hour, func(ctx context.Context) (metadata.MusicTopSongs, bool, error) {
		t, err := a.s.MusicTopSongs(ctx, mbid)
		return t, err == nil && len(t.Songs) > 0, err
	})
	if !ok {
		return nil
	}
	return &t
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
	explicit := explicitAlbums(dz.Albums)
	follows := a.albumFollows(ctx, a.person(r).UserID)
	today := time.Now().UTC().Format("2006-01-02")
	names := []nameKey{{kind: "artist", mbid: strings.ToLower(art.MBID), name: art.Name}}
	out := make([]PageAlbum, 0, len(art.Albums))
	for _, al := range art.Albums {
		if isVideo(al) {
			continue // music videos are never shown or asked for
		}
		id := strings.ToLower(al.MBID)
		p := PageAlbum{MBID: id, Name: al.Name, Type: al.PrimaryType, SecondaryTypes: listOr(al.SecondaryTypes), ReleaseDate: al.ReleaseDate,
			Disambiguation: al.Disambiguation, Shelf: shelfOf(al), Image: artPath("album", id), Explicit: explicit(al.Name),
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

// explicitAlbums says which album names Deezer marks explicit. A name with
// the same words in brackets ("Aerodynamic (Slum Village remix)") matches
// exactly; the bare name ("Aerodynamic") matches by its base only when
// every Deezer album with that base is explicit, so one explicit remix does
// not mark the clean original.
func explicitAlbums(albums []metadata.DeezerAlbum) func(name string) bool {
	full := map[string]bool{}
	base := map[string]int{} // 1: all explicit so far, -1: a clean one seen
	for _, d := range albums {
		full[fullAlbumKey(d.Title)] = d.Explicit
		k := metadata.SongKey(d.Title)
		switch {
		case !d.Explicit:
			base[k] = -1
		case base[k] == 0:
			base[k] = 1
		}
	}
	return func(name string) bool {
		if v, ok := full[fullAlbumKey(name)]; ok {
			return v
		}
		return base[metadata.SongKey(name)] == 1
	}
}

// fullAlbumKey keeps the words in brackets, unlike SongKey.
func fullAlbumKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (a *API) similarArtists(ctx context.Context, mbid string, n int) []PageArtist {
	return a.similarArtistsWait(ctx, mbid, n, musicSideWait)
}

// similarArtistsWait is similarArtists waiting up to wait for a list not
// kept yet.
func (a *API) similarArtistsWait(ctx context.Context, mbid string, n int, wait time.Duration) []PageArtist {
	out := []PageArtist{}
	if a.s.MusicSimilar == nil {
		return out
	}
	// A list not kept yet is looked up behind the answer (similarTo goes on).
	list := a.similarTo([]string{mbid}, wait)[mbid]
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
	a.prefetchSimilarArt(out)
	return out
}

// prefetchSimilarArt fetches every similar artist's picture into the
// artwork cache in the background, through the same chain as the other
// music covers, so "Fans also like" arrives without placeholders.
func (a *API) prefetchSimilarArt(list []PageArtist) {
	if len(list) == 0 || !a.partIsOn(parts.Music) || a.s.Library == nil || a.s.Library.Artwork == nil {
		return
	}
	items := make([]metadata.MusicTrendItem, 0, len(list))
	for _, p := range list {
		items = append(items, metadata.MusicTrendItem{MBID: p.MBID, Name: p.Name})
	}
	a.music.prefetching.Add(1)
	go func() {
		defer a.music.prefetching.Done()
		a.prefetchMusicArt("artist", items)
	}()
}

func (a *API) artistPage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("part") {
	case "core":
		a.artistCore(w, r)
		return
	case "similar":
		a.artistSimilar(w, r)
		return
	case "more":
		a.artistMore(w, r)
		return
	case "video":
		a.artistVideo(w, r)
		return
	case "videos":
		a.artistVideoList(w, r)
		return
	}
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
	by := time.Now().Add(musicPageWait)
	// Genres, links, similar artists and top songs need only the id: each
	// starts now, with its own time limit, while the artist is looked up.
	var genres []string
	var links []metadata.ArtistLink
	var top *metadata.MusicTopSongs
	similar := []PageArtist{}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		side, cancel := context.WithTimeout(ctx, musicSideWait)
		defer cancel()
		genres, links = a.artistTags(side, mbid)
	}()
	go func() { defer wg.Done(); similar = a.similarArtists(ctx, mbid, 18) }()
	go func() {
		defer wg.Done()
		side, cancel := context.WithTimeout(ctx, musicSideWait)
		defer cancel()
		top = a.artistTopSongs(side, mbid)
	}()
	art, err, answered := waitFlight[metadata.Artist](ctx, pageFlight(a, ctx, "artist:"+mbid, func(ctx context.Context) (metadata.Artist, error) {
		return a.s.Library.Artists.Artist(metadata.QuickArtist(ctx, 1500*time.Millisecond), mbid)
	}), by)
	if errors.Is(err, metadata.ErrArtistNotFound) {
		wg.Wait()
		fail(w, http.StatusNotFound, "no artist with that id")
		return
	}
	// A slow or failing source never fails the page: it answers with what
	// it has (incomplete) and the lookup goes on, kept for the next ask.
	incomplete := !answered || err != nil
	if incomplete {
		art = metadata.Artist{MBID: mbid, Name: a.knownName("artist", mbid)}
	}
	var dz metadata.DeezerProfile
	var bio *metadata.WikiSummary
	var store []metadata.MusicStoreAlbum
	var storeGap *PageGap
	side, cancel := context.WithTimeout(ctx, max(time.Until(by), musicSideFloor))
	defer cancel()
	if a.s.MusicDiscography != nil && len(art.Albums) == 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := a.s.MusicDiscography(side, mbid)
			if err == nil {
				store = d.Albums
			} else {
				storeGap = gapOf(err)
			}
		}()
	}
	wg.Add(2)
	go func() { defer wg.Done(); dz = a.deezerProfile(side, mbid, art) }()
	go func() { defer wg.Done(); bio = a.artistBio(side, mbid, art.Name) }()
	wg.Wait()
	albums, t, err := a.pageAlbums(ctx, r, art, dz)
	if err != nil {
		internal(w, err)
		return
	}
	v := ArtistPageView{MBID: mbid, Name: art.Name, Type: art.Type, Country: art.Country, Disambiguation: art.Disambiguation, Status: art.Status,
		Genres: listOr(genres), Image: artPath("artist", mbid), Bio: bio, Similar: similar, Related: []PageArtist{}, Top: []PageSong{}, Kid: kid,
		Missing: PageMissing{Albums: []string{}}}
	if t != nil {
		v.TitleID = t.ID
		if img := artistImage(*t); img != "" {
			v.Image = img
		}
		if v.Name == "" {
			v.Name = t.Name
		}
	}
	v.Incomplete = incomplete
	if incomplete {
		v.Gap = gapOf(err)
		if v.Gap == nil && len(art.Albums) == 0 && len(store) == 0 {
			v.Gap = storeGap
		}
		if v.Gap != nil {
			v.Incomplete = true
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
			if !al.Upcoming && (al.State == "" || al.State == "not_included") {
				v.Missing.Count++
				v.Missing.Albums = append(v.Missing.Albums, al.MBID)
				if al.Shelf == "albums" {
					v.Missing.SizeBytes += missingAlbumBytes
				} else {
					v.Missing.SizeBytes += missingSingleBytes
				}
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
	v.Popularity = art.Popularity
	a.prefetchTracklists(ctx, firstAlbums(v))
	var songNames []nameKey
	// The metadata server's top songs feed the shelf; with none known (or it
	// not answering in time) Deezer's list stands in.
	songs := dz.Top
	if top != nil {
		songs = songs[:0:0]
		for _, s := range top.Songs {
			songs = append(songs, metadata.DeezerTrack{Title: s.Title, LengthMS: s.LengthMS, Explicit: s.Explicit, Album: s.Album, Cover: s.Cover})
		}
	}
	if len(kept) == 0 {
		for _, al := range store {
			if (kid && al.Explicit) || strings.TrimSpace(al.Title) == "" {
				continue
			}
			x := PageStoreAlbum{Title: al.Title, ReleaseDate: al.ReleaseDate, Type: al.Type, TrackCount: al.TrackCount, Explicit: al.Explicit}
			if al.Cover != "" {
				id := songArtID(art.Name, al.Title, al.Cover)
				x.Image = artPath("album", id)
				songNames = append(songNames, nameKey{kind: "album", mbid: id, name: al.Title, artist: art.Name, cover: al.Cover})
			}
			v.StoreAlbums = append(v.StoreAlbums, x)
		}
	}
	for _, s := range songs {
		if kid && s.Explicit {
			continue
		}
		song := PageSong{Title: s.Title, LengthMS: s.LengthMS, Explicit: s.Explicit, Album: s.Album, Image: s.Cover}
		if al, ok := songAlbum(byName, s.Album); ok {
			song.AlbumMBID, song.Image = al.MBID, al.Image
		} else {
			// A song whose release is not in the discography (a feature, a
			// single on someone else's record): the artist's photo, never a
			// letter tile.
			song.Image = artPath("artist", mbid)
		}
		v.Top = append(v.Top, song)
	}
	a.rememberMusicNames(songNames)
	var names []nameKey
	for _, l := range links {
		names = append(names, nameKey{kind: "artist", mbid: l.MBID, name: l.Name})
		v.Related = append(v.Related, PageArtist{MBID: l.MBID, Name: l.Name, Image: artPath("artist", l.MBID), Note: l.Type})
	}
	a.rememberMusicNames(names)
	a.warmMusicArt(v)
	musicPageCaching(w, v.Incomplete)
	a.writeMusicJSON(w, r, v)
}

// tracklistPrefetchMax caps the albums whose tracklists an artist page asks
// ahead, and tracklistPrefetchAtOnce how many are asked at a time.
const (
	tracklistPrefetchMax    = 8
	tracklistPrefetchAtOnce = 2
)

// firstAlbums are the albums an artist page shows first: the latest
// release, the first albums, then the first singles.
func firstAlbums(v ArtistPageView) []string {
	var out []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] && len(out) < tracklistPrefetchMax {
			seen[id] = true
			out = append(out, id)
		}
	}
	if v.Latest != nil {
		add(v.Latest.MBID)
	}
	for _, shelf := range []string{"albums", "singles"} {
		n := 0
		for _, al := range v.Albums {
			if al.Shelf == shelf && !al.Upcoming && n < 6 {
				add(al.MBID)
				n++
			}
		}
	}
	return out
}

// prefetchTracklists asks the tracklists of the given albums in the
// background (the metadata server's one-call answer only), under the same
// flight an album page joins, so a click finds the list kept or on its way.
func (a *API) prefetchTracklists(ctx context.Context, ids []string) {
	if len(ids) == 0 || a.s.Library == nil || a.s.Library.Artists == nil {
		return
	}
	base := metadata.TracklistOnly(context.WithoutCancel(ctx))
	go func() {
		sem := make(chan struct{}, tracklistPrefetchAtOnce)
		var wg sync.WaitGroup
		for _, id := range ids {
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer func() { <-sem; wg.Done() }()
				fl := pageFlight(a, base, "pressings:"+id, func(ctx context.Context) ([]metadata.Pressing, error) {
					return a.s.Library.Artists.Pressings(ctx, id)
				})
				<-fl.done
			}()
		}
		wg.Wait()
	}()
}

const (
	// musicSideWait caps each optional part of a page (tags, top songs).
	// A part kept answers at once; one not kept goes on behind the answer.
	musicSideWait = 800 * time.Millisecond
	// musicLookupMax bounds a lookup that goes on after the page answered
	// (pageFlight).
	musicLookupMax = time.Minute
)

// musicPageWait caps the essential lookup of an artist or album page: past
// it the page answers with what it has (incomplete:true) and the lookup goes
// on in the background, joined by the next ask (tests shorten it).
var musicPageWait = 800 * time.Millisecond

// musicSideFloor is the least time the page's kept side parts (Deezer,
// bio, discography) get to answer; ones not kept go on behind the answer.
const musicSideFloor = 300 * time.Millisecond

// musicPageCaching lets the browser keep a full page a minute; an
// incomplete one is asked again.
func musicPageCaching(w http.ResponseWriter, incomplete bool) {
	if incomplete {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
}

// warmArtMax caps the pictures one page view warms: each cold one is a
// metadata server call, and an install may make 120 a minute, so warming
// a whole discography would make the page's own lookups wait.
const warmArtMax = 28

// warmMusicArt starts fetching the pictures the page shows first (the
// artist, the latest release, the top songs' albums, the first albums and
// singles, the first similar artists) a few at a time, so they are kept
// before the browser asks for each. The rest load as the person scrolls.
func (a *API) warmMusicArt(v ArtistPageView) {
	// The whole page's artists and covers are stored by the low-priority
	// pool, so the next click (a similar or related artist) is instant.
	var all []string
	for _, p := range v.Similar {
		all = append(all, p.Image)
	}
	for _, p := range v.Related {
		all = append(all, p.Image)
	}
	for _, al := range v.Albums {
		all = append(all, al.Image)
	}
	a.warmArtPaths(all...)
	const pre = "/api/v1/music-trending/art/"
	var keys []string
	seen := map[string]bool{}
	add := func(img string) {
		if rest, ok := strings.CutPrefix(img, pre); ok && len(keys) < warmArtMax && !seen[rest] {
			seen[rest] = true
			keys = append(keys, rest)
		}
	}
	add(v.Image)
	if v.Latest != nil {
		add(v.Latest.Image)
	}
	for _, s := range v.Top {
		add(s.Image)
	}
	for _, shelf := range []string{"albums", "singles"} {
		n := 0
		for _, al := range v.Albums {
			if al.Shelf == shelf && !al.Upcoming && n < 8 {
				add(al.Image)
				n++
			}
		}
	}
	for i, p := range v.Similar {
		if i < 6 {
			add(p.Image)
		}
	}
	if len(keys) == 0 || a.s.Library == nil || a.s.Library.Artwork == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		// Three at a time: the metadata server takes eight calls at once per
		// install, and the page's own lookups must not wait behind pictures
		// (a refused lookup waits a second, then falls back to MusicBrainz).
		sem := make(chan struct{}, 3)
		var wg sync.WaitGroup
		for _, k := range keys {
			kind, mbid, _ := strings.Cut(k, "/")
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				a.musicTrendArtFile(ctx, kind, mbid, "500")
			}()
		}
		wg.Wait()
	}()
}

func (a *API) albumPage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("part") {
	case "core":
		a.albumCore(w, r)
		return
	case "near":
		a.albumNear(w, r)
		return
	}
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if artist != "" && !metadata.ValidMBID(artist) {
		artist = "" // a bad ?artist= never blocks the page: the album names its own artist
	}
	if !metadata.ValidMBID(mbid) {
		fail(w, http.StatusBadRequest, "That album link is not valid.")
		return
	}
	if !a.musicPageOpen(w) {
		return
	}
	kid := a.kidMax(r) != ""
	ctx := metadata.MemberLookup(r.Context())
	// The pressings need only the album id: ask them while the artist is
	// looked up (a bare link's artist too). Both go on after the page stopped waiting (kept for the
	// next ask).
	psFlight := pageFlight(a, ctx, "pressings:"+mbid, func(ctx context.Context) ([]metadata.Pressing, error) {
		return a.s.Library.Artists.Pressings(ctx, mbid)
	})
	if artist == "" {
		// A bare album link: find the artist from the album itself.
		if artist = a.albumArtistOf(ctx, mbid); artist == "" {
			fail(w, http.StatusNotFound, "We could not find this album.")
			return
		}
	}
	by := time.Now().Add(musicPageWait)
	simDone := make(chan []PageArtist, 1)
	go func() {
		if r.URL.Query().Get("similar") == "0" {
			simDone <- []PageArtist{}
			return
		}
		simDone <- a.similarArtists(ctx, artist, 14)
	}()
	art, err, answered := waitFlight[metadata.Artist](ctx, pageFlight(a, ctx, "album-artist:"+artist+"|"+mbid, func(ctx context.Context) (metadata.Artist, error) {
		art, err := a.s.Library.Artists.Artist(metadata.QuickArtist(ctx, 1500*time.Millisecond), artist)
		if err == nil && art.Partial && !hasAlbum(art, mbid) {
			art, err = a.s.Library.Artists.Artist(metadata.WholeArtist(ctx), artist)
		}
		return art, err
	}), by)
	if errors.Is(err, metadata.ErrArtistNotFound) {
		if a.albumWithRealArtist(ctx, w, r, mbid, artist) {
			return
		}
		fail(w, http.StatusNotFound, "We could not find this album.")
		return
	}
	// A slow or failing source never fails the page: the album is drawn
	// from the names already kept and the page says it is incomplete.
	incomplete := !answered || err != nil
	if incomplete {
		art = metadata.Artist{MBID: artist, Name: a.knownName("artist", artist)}
	}
	side, cancel := context.WithTimeout(ctx, max(time.Until(by), musicSideFloor))
	defer cancel()
	dz := a.deezerProfile(side, artist, art)
	albums, t, err := a.pageAlbums(ctx, r, art, dz)
	if err != nil {
		internal(w, err)
		return
	}
	if incomplete && t != nil && art.Name == "" {
		art.Name = t.Name
	}
	var me *PageAlbum
	for i := range albums {
		if albums[i].MBID == mbid {
			me = &albums[i]
		}
	}
	if me == nil && incomplete {
		x := PageAlbum{MBID: mbid, Name: a.knownName("album", mbid), SecondaryTypes: []string{}, Image: artPath("album", mbid), Shelf: "albums"}
		me = &x
	}
	if me == nil {
		if a.albumWithRealArtist(ctx, w, r, mbid, artist) {
			return
		}
		fail(w, http.StatusNotFound, "We could not find this album.")
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
	ps, psErr, psAnswered := waitFlight[[]metadata.Pressing](ctx, psFlight, by)
	if !psAnswered && psErr == nil {
		psErr = context.DeadlineExceeded // still asking: the next ask joins it
	}
	if incomplete || psErr != nil && len(ps) == 0 {
		v.Incomplete = true
		if len(ps) == 0 {
			if v.Gap = gapOf(psErr); v.Gap == nil {
				v.Gap = gapOf(err)
			}
		}
	}
	main := mainPressing(ps)
	// Deezer's album: label and explicit marks per track.
	var dzInfo metadata.DeezerAlbumInfo
	// The same name with the same words in brackets first, else the same base name.
	pick := int64(0)
	for _, d := range dz.Albums {
		if fullAlbumKey(d.Title) == fullAlbumKey(me.Name) {
			pick = d.ID
			break
		}
		if pick == 0 && metadata.SongKey(d.Title) == metadata.SongKey(me.Name) {
			pick = d.ID
		}
	}
	if pick != 0 {
		dzInfo = a.deezerAlbum(side, pick)
	}
	v.Label = dzInfo.Label
	have := map[string]string{}
	if me.PartID != 0 {
		have = a.trackStates(ctx, me.PartID)
	}
	if main != nil {
		for _, tr := range main.Tracks {
			pt := PageTrack{Disc: tr.Disc, Number: tr.Number, Name: tr.Name, LengthMS: tr.LengthMS, RecordingID: tr.RecordingID, Explicit: dzInfo.Tracks[metadata.SongKey(tr.Name)]}
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
	v.Editions = editions(ps)
	for i := range v.Editions {
		v.Editions[i].List = editionTracks(ps, v.Editions[i].MBID, have, me.PartID != 0)
	}
	for _, al := range albums {
		if al.MBID != mbid && (al.Shelf == "albums" || al.Shelf == "singles") && !(kid && al.Explicit) && len(v.MoreBy) < 20 {
			v.MoreBy = append(v.MoreBy, al)
		}
	}
	select {
	case v.Similar = <-simDone:
	case <-time.After(max(time.Until(by), 0)):
		v.Similar = []PageArtist{}
	}
	// The lower row's covers (More by, Fans also like) are fetched now, so
	// they arrive with the page instead of one by one.
	a.warmMusicArt(ArtistPageView{Image: v.Artist.Image, Albums: v.MoreBy, Similar: v.Similar})
	musicPageCaching(w, v.Incomplete)
	a.writeMusicJSON(w, r, v)
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

// editionWords mark a disambiguation that names an edition a person
// would choose (deluxe, remaster); others name a pressing plant, a
// country or a catalogue number and are left off.
var editionWords = []string{"deluxe", "edition", "remaster", "anniversary", "expanded", "collector", "bonus", "special", "version", "mono", "stereo",
	"clean", "explicit", "box", "super", "complete", "tour", "reissue", "japan", "live", "acoustic", "instrumental", "demo"}

func editionLabel(p metadata.Pressing) string {
	l := p.Name
	d := strings.ToLower(p.Disambiguation)
	for _, w := range editionWords {
		if strings.Contains(d, w) {
			return l + " (" + p.Disambiguation + ")"
		}
	}
	return l
}

// editions lists the album's official editions: one per name and track
// count (pressings of the same edition fold into one, with the earliest
// date), the earliest first.
func editions(ps []metadata.Pressing) []PageEdition {
	out := []PageEdition{}
	at := map[string]int{}
	for _, p := range ps {
		if p.Status != "" && p.Status != "official" || len(p.Tracks) == 0 {
			continue
		}
		l := editionLabel(p)
		k := fullAlbumKey(l) + "|" + strconv.Itoa(len(p.Tracks))
		if i, ok := at[k]; ok {
			if p.Date != "" && (out[i].Date == "" || p.Date < out[i].Date) {
				out[i].Date, out[i].MBID = p.Date, p.MBID
			}
			continue
		}
		at[k] = len(out)
		out = append(out, PageEdition{MBID: p.MBID, Label: l, Tracks: len(p.Tracks), Date: p.Date})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Date, out[j].Date
		if a == "" || b == "" {
			return a != ""
		}
		return a < b
	})
	return out
}

// ArtistCoreView answers GET /api/v1/music/artists/{mbid}?part=core: what the
// hero needs (name, picture, facts, bio, the Library's artist), answered
// from what is kept or one quick lookup. The shelves come from the full
// page (?similar=0) and ?part=similar, asked at the same time.
type ArtistCoreView struct {
	MBID           string                `json:"mbid"`
	Name           string                `json:"name"`
	Type           string                `json:"type,omitempty"`
	Country        string                `json:"country,omitempty"`
	Disambiguation string                `json:"disambiguation,omitempty"`
	Status         string                `json:"status,omitempty"`
	Image          string                `json:"image"`
	TitleID        int64                 `json:"title_id,omitempty"`
	Bio            *metadata.WikiSummary `json:"bio"`
	Kid            bool                  `json:"kid"`
	// Albums is the metadata server's complete stored discography (names,
	// kinds, dates, covers), so the shelves draw with the hero; the full
	// answer adds Library state and explicit marks. None when the list is
	// not complete there.
	Albums []PageAlbum `json:"albums,omitempty"`
}

// coreWarmMax caps the covers the core answer warms: the first grid (18)
// only, so the install's allowance at the metadata server goes to what is
// on screen.
const coreWarmMax = 18

// coreAlbums lists a complete stored discography for the core answer, newest first.
func coreAlbums(art metadata.Artist) []PageAlbum {
	if art.Partial || len(art.Albums) == 0 {
		return nil
	}
	today := time.Now().UTC().Format("2006-01-02")
	out := make([]PageAlbum, 0, len(art.Albums))
	for _, al := range art.Albums {
		if isVideo(al) {
			continue
		}
		id := strings.ToLower(al.MBID)
		out = append(out, PageAlbum{MBID: id, Name: al.Name, Type: al.PrimaryType, SecondaryTypes: listOr(al.SecondaryTypes), ReleaseDate: al.ReleaseDate,
			Disambiguation: al.Disambiguation, Shelf: shelfOf(al), Image: artPath("album", id),
			Upcoming: len(al.ReleaseDate) == 10 && al.ReleaseDate > today})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ReleaseDate > out[j].ReleaseDate })
	return out
}

// coreBioWait is how long, from the request, the hero waits for a bio not
// kept yet; the full page brings it when this runs out.
const coreBioWait = 450 * time.Millisecond

// knownName is a name the pages or Explore already showed for this id.
func (a *API) knownName(kind, mbid string) string {
	c := &a.music
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.names[kind+"|"+mbid].Name
}

// albumArtistOf names an album's artist when the link did not: from the
// names Explore kept, else from the music source.
func (a *API) albumArtistOf(ctx context.Context, mbid string) string {
	c := &a.music
	c.mu.Lock()
	id := strings.ToLower(c.names["album|"+mbid].ArtistMBID)
	c.mu.Unlock()
	if metadata.ValidMBID(id) {
		return id
	}
	if f, ok := a.s.Library.Artists.(metadata.AlbumArtistFinder); ok {
		if id, err := f.AlbumArtist(ctx, mbid); err == nil {
			return id
		}
	}
	return ""
}

// albumWithRealArtist answers the album page again for the artist the album
// itself names, when the link's artist is wrong. False when there is no other artist.
func (a *API) albumWithRealArtist(ctx context.Context, w http.ResponseWriter, r *http.Request, mbid, artist string) bool {
	real := a.albumArtistOf(ctx, mbid)
	if real == "" || real == artist {
		return false
	}
	q := r.URL.Query()
	q.Set("artist", real)
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	r2.URL = &u
	a.albumPage(w, r2)
	return true
}

// musicPageGate checks the person, the id and that music is on.
func (a *API) musicPageGate(w http.ResponseWriter, r *http.Request) (string, bool) {
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if a.kidMax(r) != "" {
		fail(w, http.StatusForbidden, aboveLimit)
		return "", false
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	if !metadata.ValidMBID(mbid) {
		fail(w, http.StatusBadRequest, "mbid must be a MusicBrainz id")
		return "", false
	}
	return mbid, a.musicPageOpen(w)
}

func (a *API) artistCore(w http.ResponseWriter, r *http.Request) {
	mbid, ok := a.musicPageGate(w, r)
	if !ok {
		return
	}
	ctx := metadata.MemberLookup(r.Context())
	bioBy := time.After(coreBioWait)
	t, err := a.libraryArtist(ctx, mbid)
	if err != nil {
		internal(w, err)
		return
	}
	v := ArtistCoreView{MBID: mbid, Image: artPath("artist", mbid)}
	if t != nil {
		v.TitleID, v.Name = t.ID, t.Name
		if img := artistImage(*t); img != "" {
			v.Image = img
		}
	}
	if v.Name == "" {
		v.Name = a.knownName("artist", mbid)
	}
	// The bio needs only the name: start it at once when the name is known.
	bioDone := make(chan *metadata.WikiSummary, 1)
	startBio := func(name string) {
		go func() {
			b, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
			defer cancel()
			bioDone <- a.artistBio(b, mbid, name)
		}()
	}
	if v.Name != "" {
		startBio(v.Name)
	}
	// Wait 0: the metadata server's answer (kept for popular artists); the
	// full MusicBrainz list goes on for the full page. With the name known
	// already, the facts (kind, country) may come with the full page instead.
	wait := 3 * time.Second
	if v.Name != "" {
		wait = 300 * time.Millisecond
	}
	quick, cancel := context.WithTimeout(metadata.QuickArtist(ctx, 0), wait)
	defer cancel()
	art, err := a.s.Library.Artists.Artist(quick, mbid)
	switch {
	case errors.Is(err, metadata.ErrArtistNotFound):
		fail(w, http.StatusNotFound, "no artist with that id")
		return
	case err != nil && v.Name == "":
		fail(w, http.StatusBadGateway, "the music source did not answer; try again shortly")
		return
	case err == nil:
		if v.Name == "" {
			startBio(art.Name)
		}
		v.Name, v.Type, v.Country, v.Disambiguation, v.Status = art.Name, art.Type, art.Country, art.Disambiguation, art.Status
		if v.Albums = coreAlbums(art); len(v.Albums) > 0 {
			names := make([]nameKey, 0, len(v.Albums))
			imgs := make([]string, 0, len(v.Albums))
			for _, al := range v.Albums {
				names = append(names, nameKey{kind: "album", mbid: al.MBID, name: al.Name, artist: art.Name})
				if len(imgs) < coreWarmMax {
					imgs = append(imgs, al.Image)
				}
			}
			a.rememberMusicNames(names)
			a.warmArtPaths(imgs...)
		}
	}
	select {
	case v.Bio = <-bioDone:
	case <-bioBy:
	}
	a.rememberMusicNames([]nameKey{{kind: "artist", mbid: mbid, name: v.Name}})
	w.Header().Set("Cache-Control", "private, max-age=60")
	a.writeMusicJSON(w, r, v)
}

// artistSimilar answers GET /api/v1/music/artists/{mbid}?part=similar[&n=14].
func (a *API) artistSimilar(w http.ResponseWriter, r *http.Request) {
	mbid, ok := a.musicPageGate(w, r)
	if !ok {
		return
	}
	n := 18
	if r.URL.Query().Get("n") == "14" {
		n = 14
	}
	list := a.similarArtists(metadata.MemberLookup(r.Context()), mbid, n)
	a.warmMusicArt(ArtistPageView{Similar: list})
	if len(list) == 0 && a.similarPending(mbid) {
		// The server is still getting the list: the page asks again.
		w.Header().Set("Cache-Control", "no-store")
		a.writeMusicJSON(w, r, map[string]any{"similar": list, "incomplete": true})
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	a.writeMusicJSON(w, r, map[string]any{"similar": list})
}

// AlbumCoreView answers GET /api/v1/music/albums/{mbid}?part=core&artist=: the
// album hero (name, kind, date, cover, artist), from the metadata server's
// answer for the artist or the names the artist page and Explore kept.
type AlbumCoreView struct {
	MBID           string     `json:"mbid"`
	Name           string     `json:"name"`
	Type           string     `json:"type,omitempty"`
	SecondaryTypes []string   `json:"secondary_types"`
	ReleaseDate    string     `json:"release_date,omitempty"`
	Disambiguation string     `json:"disambiguation,omitempty"`
	Image          string     `json:"image"`
	Artist         PageArtist `json:"artist"`
	Kid            bool       `json:"kid"`
}

// albumCoreWait caps the core's wait for the artist's album list.
const albumCoreWait = 300 * time.Millisecond

// albumCoreMiss is how long an album core answered 404 answers so again.
const albumCoreMiss = time.Minute

// coreMissed says the core of mbid (by artist) answered 404 within albumCoreMiss.
func (a *API) coreMissed(mbid, artist string) bool {
	c := &a.music
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.coreMiss[mbid+"|"+artist]
	return ok && time.Since(at) < albumCoreMiss
}

func (a *API) rememberCoreMiss(mbid, artist string) {
	c := &a.music
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.coreMiss == nil || len(c.coreMiss) > 5000 {
		c.coreMiss = map[string]time.Time{}
	}
	c.coreMiss[mbid+"|"+artist] = time.Now()
}

func (a *API) albumCore(w http.ResponseWriter, r *http.Request) {
	mbid, ok := a.musicPageGate(w, r)
	if !ok {
		return
	}
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if !metadata.ValidMBID(artist) {
		fail(w, http.StatusBadRequest, "send the album's and its artist's MusicBrainz ids (?artist=)")
		return
	}
	ctx := metadata.MemberLookup(r.Context())
	if a.coreMissed(mbid, artist) {
		fail(w, http.StatusNotFound, "this album is not kept yet; the full page answers")
		return
	}
	v := AlbumCoreView{MBID: mbid, Image: artPath("album", mbid), SecondaryTypes: []string{}, Artist: PageArtist{MBID: artist, Image: artPath("artist", artist)}}
	if t, err := a.libraryArtist(ctx, artist); err == nil && t != nil {
		v.Artist.Name = t.Name
		if img := artistImage(*t); img != "" {
			v.Artist.Image = img
		}
	}
	// The names Explore and the artist page kept answer at once; the
	// metadata server adds the kind and date when it answers quickly, and
	// a lookup past that goes on behind the answer (the next ask joins it).
	art, err, answered := waitFlight[metadata.Artist](ctx, pageFlight(a, ctx, "core-artist:"+artist, func(ctx context.Context) (metadata.Artist, error) {
		return a.s.Library.Artists.Artist(metadata.QuickArtist(ctx, 0), artist)
	}), time.Now().Add(albumCoreWait))
	if answered && err == nil {
		if v.Artist.Name == "" {
			v.Artist.Name = art.Name
		}
		for _, al := range art.Albums {
			if strings.EqualFold(al.MBID, mbid) {
				v.Name, v.Type, v.SecondaryTypes, v.ReleaseDate, v.Disambiguation = al.Name, al.PrimaryType, listOr(al.SecondaryTypes), al.ReleaseDate, al.Disambiguation
			}
		}
	}
	if v.Name == "" {
		v.Name = a.knownName("album", mbid)
	}
	if v.Artist.Name == "" {
		v.Artist.Name = a.knownName("artist", artist)
	}
	if v.Name == "" {
		// Not in the kept list (a long discography): the full page answers.
		// Remembered a minute, so a page listing it does not ask each load.
		a.rememberCoreMiss(mbid, artist)
		fail(w, http.StatusNotFound, "this album is not kept yet; the full page answers")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	a.writeMusicJSON(w, r, v)
}

// editionTracks lists one edition's tracks with the Library's have per
// track (by recording id, else by name), as the main list does.
func editionTracks(ps []metadata.Pressing, mbid string, have map[string]string, kept bool) []PageTrack {
	out := []PageTrack{}
	for _, p := range ps {
		if p.MBID != mbid {
			continue
		}
		for _, tr := range p.Tracks {
			pt := PageTrack{Disc: tr.Disc, Number: tr.Number, Name: tr.Name, LengthMS: tr.LengthMS}
			if kept {
				pt.Have = "missing"
				if s, ok := have[strings.ToLower(tr.RecordingID)]; ok {
					pt.Have = s
				} else if s, ok := have["n:"+metadata.SongKey(tr.Name)]; ok {
					pt.Have = s
				}
			}
			out = append(out, pt)
		}
		break
	}
	return out
}

// musicCreditTypes are the release kinds Appears on and the neighbours'
// albums may show (never videos).
var musicCreditTypes = map[string]bool{"album": true, "ep": true, "single": true}

func creditOf(al metadata.MusicTopAlbum) PageCredit {
	id := strings.ToLower(al.MBID)
	c := PageCredit{PageAlbum: PageAlbum{MBID: id, Name: al.Title, Type: al.Type, SecondaryTypes: []string{}, ReleaseDate: al.Date, Image: artPath("album", id)}, Listeners: al.Listeners}
	c.Shelf = "albums"
	if al.Type != "album" {
		c.Shelf = "singles"
	}
	if len(al.Artists) > 0 {
		c.ArtistMBID = strings.ToLower(al.Artists[0])
	}
	if len(al.ArtistNames) > 0 {
		c.ArtistName = al.ArtistNames[0]
	}
	return c
}

// artistMore answers GET /api/v1/music/artists/{mbid}?part=more: the
// Essential albums (the artist's own albums by listeners, at most 6) and
// Appears on (releases led by another artist that credit this one), from
// the metadata server's artist data. Empty lists when it has none.
func (a *API) artistMore(w http.ResponseWriter, r *http.Request) {
	mbid, ok := a.musicPageGate(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(metadata.MemberLookup(r.Context()), 4*time.Second)
	defer cancel()
	essential, appears := []string{}, []PageCredit{}
	if d := a.musicArtistDataOf(ctx, []string{mbid}, time.Now())[mbid]; d != nil {
		own := []metadata.MusicTopAlbum{}
		seen := map[string]bool{}
		for _, al := range d.Albums {
			if !musicCreditTypes[al.Type] || seen[al.MBID] {
				continue
			}
			seen[al.MBID] = true
			lead := len(al.Artists) == 0 || strings.EqualFold(al.Artists[0], mbid)
			switch {
			case lead && al.Type == "album":
				own = append(own, al)
			case !lead && len(appears) < 20:
				appears = append(appears, creditOf(al))
			}
		}
		a.warmCredits(appears)
		sort.SliceStable(own, func(i, j int) bool { return own[i].Listeners > own[j].Listeners })
		// The Essential albums' covers go through the cover chain now, so
		// no card waits on (or misses) a cold picture.
		warm := []PageCredit{}
		for i := 0; i < len(own) && i < 6; i++ {
			essential = append(essential, strings.ToLower(own[i].MBID))
			c := creditOf(own[i])
			c.MBID = strings.ToLower(c.MBID)
			warm = append(warm, c)
		}
		a.warmCredits(warm)
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	a.writeMusicJSON(w, r, map[string]any{"essential": essential, "appears_on": appears})
}

// warmCredits lets the picture route serve these albums (their names and
// artists feed the cover chain's name search) and fetches their covers
// into the artwork cache in the background, so the shelf arrives without
// placeholders. Without the names the picture route has nothing to ask.
func (a *API) warmCredits(list []PageCredit) {
	if len(list) == 0 {
		return
	}
	names := make([]nameKey, 0, len(list))
	items := make([]metadata.MusicTrendItem, 0, len(list))
	for _, c := range list {
		names = append(names, nameKey{kind: "album", mbid: c.MBID, name: c.Name, artist: c.ArtistName})
		items = append(items, metadata.MusicTrendItem{MBID: c.MBID, Name: c.Name, Title: c.Name, Artist: c.ArtistName})
	}
	a.rememberMusicNames(names)
	if !a.partIsOn(parts.Music) || a.s.Library == nil || a.s.Library.Artwork == nil {
		return
	}
	a.music.prefetching.Add(1)
	go func() {
		defer a.music.prefetching.Done()
		a.prefetchMusicArt("album", items)
	}()
}

// albumNearFresh is how long an album's "more like this" list (an empty
// one too) is answered before it is asked again behind the answer.
const albumNearFresh = 6 * time.Hour

// albumNearOf is the best album of each artist similar to the album's
// artist (the album itself left out), waiting on the similar list as long
// as ctx allows.
func (a *API) albumNearOf(ctx context.Context, mbid, artist string) []PageCredit {
	near := []PageCredit{}
	wait := musicSideWait
	if d, ok := ctx.Deadline(); ok {
		wait = max(time.Until(d)-time.Second, musicSideWait)
	}
	sim := a.similarArtistsWait(ctx, artist, 14, wait)
	ids := make([]string, 0, len(sim))
	for _, p := range sim {
		ids = append(ids, p.MBID)
	}
	data := a.musicArtistDataOf(ctx, ids, time.Now())
	for _, p := range sim {
		d := data[p.MBID]
		if d == nil {
			continue
		}
		var best *metadata.MusicTopAlbum
		for i, al := range d.Albums {
			if al.Type != "album" || strings.EqualFold(al.MBID, mbid) || len(al.Artists) > 0 && !strings.EqualFold(al.Artists[0], p.MBID) {
				continue
			}
			if best == nil || al.Listeners > best.Listeners {
				best = &d.Albums[i]
			}
		}
		if best != nil {
			c := creditOf(*best)
			c.ArtistMBID, c.ArtistName = p.MBID, p.Name
			near = append(near, c)
		}
	}
	return near
}

// albumNear answers GET /api/v1/music/albums/{mbid}?part=near&artist=: the
// top album (by listeners) of each artist like this one, at most 14.
func (a *API) albumNear(w http.ResponseWriter, r *http.Request) {
	mbid, ok := a.musicPageGate(w, r)
	if !ok {
		return
	}
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if !metadata.ValidMBID(artist) {
		fail(w, http.StatusBadRequest, "send the album's and its artist's MusicBrainz ids (?artist=)")
		return
	}
	ctx, cancel := context.WithTimeout(metadata.MemberLookup(r.Context()), 2*musicSideWait)
	defer cancel()
	// Kept (an empty list too) and answered at once; refreshed in the
	// background after albumNearFresh. A cold lookup goes on behind the
	// answer and is kept for the next visit.
	near, _ := sideGet(a, ctx, "near", mbid+"|"+artist, albumNearFresh, func(ctx context.Context) ([]PageCredit, bool, error) {
		return a.albumNearOf(ctx, mbid, artist), true, nil
	})
	if near == nil {
		near = []PageCredit{}
	}
	a.warmCredits(near)
	w.Header().Set("Cache-Control", "private, max-age=300")
	a.writeMusicJSON(w, r, map[string]any{"near": near})
}
