package music

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
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
	Upcoming    bool   `json:"upcoming,omitempty"`
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
	Related     []PageArtist     `json:"related"`
	Kid         bool             `json:"kid"`
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
func (h *Handlers) artistTags(ctx context.Context, mbid string) ([]string, []metadata.ArtistLink) {
	src, ok := h.genreProvider()
	if !ok {
		return nil, nil
	}
	t, _ := SideGet(h, ctx, "tags", strings.ToLower(mbid), 12*time.Hour, func(ctx context.Context) (pageTags, bool, error) {
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

func (h *Handlers) musicPageRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/music/artists/{mbid}", h.artistPage)
	m.HandleFunc("GET /api/v1/music/albums/{mbid}", h.albumPage)
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

func (h *Handlers) musicPageOpen(w http.ResponseWriter) bool {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) {
		webserver.Fail(w, http.StatusNotFound, "music is turned off")
		return false
	}
	if h.lib() == nil || h.lib().Artists == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "no music source is set up")
		return false
	}
	return true
}

// artistIDs reads an artist's Deezer and Wikidata ids from its MusicBrainz
// links, fresh a day (misses an hour).
func (h *Handlers) artistIDs(ctx context.Context, mbid string) metadata.ArtistIDs {
	mbid = strings.ToLower(mbid)
	if h.lib() == nil {
		return metadata.ArtistIDs{}
	}
	src, ok := h.lib().Artists.(metadata.ArtistIDSource)
	if !ok {
		return metadata.ArtistIDs{}
	}
	ids, _ := SideGet(h, ctx, "ids", mbid, 24*time.Hour, func(ctx context.Context) (metadata.ArtistIDs, bool, error) {
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
func (h *Handlers) deezerProfile(ctx context.Context, mbid string, art metadata.Artist) metadata.DeezerProfile {
	if h.MusicDeezer != nil && !h.Deps.own() {
		// The metadata server's kept match (by the artist's own links, else
		// a checked name search there); a miss is asked again after an hour.
		p, _ := SideGet(h, ctx, "deezer2", strings.ToLower(mbid), 12*time.Hour, func(ctx context.Context) (metadata.DeezerProfile, bool, error) {
			p, err := h.MusicDeezer(ctx, mbid)
			return p, err == nil && p.ID != 0, err
		})
		return p
	}
	pics := h.pictures()
	if pics == nil {
		return metadata.DeezerProfile{}
	}
	name := art.Name
	albums := make([]string, 0, len(art.Albums))
	for _, al := range art.Albums {
		albums = append(albums, al.Name)
	}
	p, _ := SideGet(h, ctx, "deezer", strings.ToLower(mbid), 12*time.Hour, func(ctx context.Context) (metadata.DeezerProfile, bool, error) {
		if name == "" {
			return metadata.DeezerProfile{}, false, errors.New("no artist name yet")
		}
		p, err := pics.ProfileFor(ctx, name, h.artistIDs(ctx, mbid).Deezer, albums)
		return p, err == nil && p.ID != 0, err
	})
	return p
}

// artistBio is the artist's Wikipedia summary, fresh a day (none: a miss,
// asked again after an hour): the metadata server's (the article the
// artist's Wikidata item links), else the developer stand-in's by name.
func (h *Handlers) artistBio(ctx context.Context, mbid, name string) *metadata.WikiSummary {
	if h.MusicArtistBio != nil && !h.Deps.own() {
		b, _ := SideGet(h, ctx, "bio2", strings.ToLower(mbid), 24*time.Hour, func(ctx context.Context) (*metadata.WikiSummary, bool, error) {
			b, err := h.MusicArtistBio(ctx, mbid)
			return b, err == nil && b != nil, err
		})
		return b
	}
	pics := h.pictures()
	if pics == nil || strings.TrimSpace(name) == "" {
		return nil
	}
	b, _ := SideGet(h, ctx, "bio", strings.ToLower(name), 24*time.Hour, func(ctx context.Context) (*metadata.WikiSummary, bool, error) {
		b, err := pics.ArtistBio(ctx, name)
		return b, err == nil && b != nil, err
	})
	return b
}

// deezerAlbum is one Deezer album's label and per-track explicit marks:
// the metadata server's, kept here 14 days (a miss an hour), else the
// developer stand-in's.
func (h *Handlers) deezerAlbum(ctx context.Context, id int64) metadata.DeezerAlbumInfo {
	if h.MusicDeezerAlbum != nil && !h.Deps.own() {
		type kept struct {
			Label    string          `json:"label"`
			Explicit bool            `json:"explicit"`
			Tracks   map[string]bool `json:"tracks"`
		}
		k, ok := SideGet(h, ctx, "dzalbum", strconv.FormatInt(id, 10), 14*24*time.Hour, func(ctx context.Context) (kept, bool, error) {
			info, err := h.MusicDeezerAlbum(ctx, id)
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
	pics := h.pictures()
	if pics == nil {
		return metadata.DeezerAlbumInfo{Tracks: map[string]bool{}}
	}
	info, _ := pics.AlbumInfo(ctx, id)
	return info
}

// artistTopSongs is the metadata server's top songs of the artist, fresh 6
// hours; nil when it has none or did not answer (the Deezer list stands in).
func (h *Handlers) artistTopSongs(ctx context.Context, mbid string) *metadata.MusicTopSongs {
	if h.MusicTopSongs == nil {
		return nil
	}
	t, ok := SideGet(h, ctx, "top", strings.ToLower(mbid), 6*time.Hour, func(ctx context.Context) (metadata.MusicTopSongs, bool, error) {
		t, err := h.MusicTopSongs(ctx, mbid)
		return t, err == nil && len(t.Songs) > 0, err
	})
	if !ok {
		return nil
	}
	return &t
}

// libraryArtist finds the Library's artist with this MusicBrainz id.
func (h *Handlers) libraryArtist(ctx context.Context, mbid string) (*mediafiles.Title, error) {
	ts, err := database.Select[mediafiles.Title](ctx, h.Store.Read(), "titles",
		"kind = 'artist' AND state != 'removed' AND lower(json_extract(source_ids, '$.musicbrainz')) = ? LIMIT 1", mbid)
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	return &ts[0], nil
}

// pageLibraryAlbums maps album MBIDs to the Library's state and part id.
func (h *Handlers) pageLibraryAlbums(ctx context.Context, titleID int64) (map[string]library.AlbumSummary, map[int64]bool, error) {
	db := h.Store.Read()
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

func (h *Handlers) albumFollows(ctx context.Context, user int64) map[string]bool {
	out := map[string]bool{}
	rows, err := h.Store.Read().QueryContext(ctx, "SELECT mbid FROM album_follows WHERE user_id = ?", user)
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
func (h *Handlers) pageAlbums(ctx context.Context, r *http.Request, art metadata.Artist, dz metadata.DeezerProfile) ([]PageAlbum, *mediafiles.Title, error) {
	t, err := h.libraryArtist(ctx, strings.ToLower(art.MBID))
	if err != nil {
		return nil, nil, err
	}
	var have map[string]library.AlbumSummary
	var busy map[int64]bool
	if t != nil {
		if have, busy, err = h.pageLibraryAlbums(ctx, t.ID); err != nil {
			return nil, nil, err
		}
	}
	explicit := explicitAlbums(dz.Albums)
	follows := h.albumFollows(ctx, h.Person(r).UserID)
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
	h.rememberMusicNames(names)
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

func (h *Handlers) similarArtists(ctx context.Context, mbid string, n int) []PageArtist {
	return h.similarArtistsWait(ctx, mbid, n, musicSideWait)
}

// similarArtistsWait is similarArtists waiting up to wait for a list not
// kept yet.
func (h *Handlers) similarArtistsWait(ctx context.Context, mbid string, n int, wait time.Duration) []PageArtist {
	out := []PageArtist{}
	if h.MusicSimilar == nil {
		return out
	}
	// A list not kept yet is looked up behind the answer (similarTo goes on).
	list := h.similarTo([]string{mbid}, wait)[mbid]
	var names []nameKey
	for _, s := range list {
		if len(out) >= n || !metadata.ValidMBID(s.MBID) {
			continue
		}
		id := strings.ToLower(s.MBID)
		names = append(names, nameKey{kind: "artist", mbid: id, name: s.Name})
		out = append(out, PageArtist{MBID: id, Name: s.Name, Image: artPath("artist", id)})
	}
	h.rememberMusicNames(names)
	h.prefetchSimilarArt(out)
	return out
}

// prefetchSimilarArt fetches every similar artist's picture into the
// artwork cache in the background, through the same chain as the other
// music covers, so "Fans also like" arrives without placeholders.
func (h *Handlers) prefetchSimilarArt(list []PageArtist) {
	if len(list) == 0 || !h.PartIsOn(parts.Music) || h.lib() == nil || h.lib().Artwork == nil {
		return
	}
	items := make([]metadata.MusicTrendItem, 0, len(list))
	for _, p := range list {
		items = append(items, metadata.MusicTrendItem{MBID: p.MBID, Name: p.Name})
	}
	h.music.prefetching.Add(1)
	go func() {
		defer h.music.prefetching.Done()
		h.prefetchMusicArt("artist", items)
	}()
}

// pictures is the direct Deezer and Wikipedia client: the owner's (Your own
// keys), else the developer stand-in; nil when neither.
func (h *Handlers) pictures() *metadata.ArtistImages {
	if h.Deps.own() {
		return h.OwnPictures
	}
	if h.lib() == nil {
		return nil
	}
	return h.lib().ArtistImages
}
