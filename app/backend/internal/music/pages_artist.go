package music

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func (h *Handlers) artistPage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("part") {
	case "core":
		h.artistCore(w, r)
		return
	case "similar":
		h.artistSimilar(w, r)
		return
	case "more":
		h.artistMore(w, r)
		return
	case "video":
		h.artistVideo(w, r)
		return
	case "videos":
		h.artistVideoList(w, r)
		return
	}
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if h.KidMax(r) != "" {
		webserver.Fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	if !metadata.ValidMBID(mbid) {
		webserver.Fail(w, http.StatusBadRequest, "mbid must be a MusicBrainz artist id")
		return
	}
	if !h.musicPageOpen(w) {
		return
	}
	kid := h.KidMax(r) != ""
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
		genres, links = h.artistTags(side, mbid)
	}()
	go func() { defer wg.Done(); similar = h.similarArtists(ctx, mbid, 18) }()
	go func() {
		defer wg.Done()
		side, cancel := context.WithTimeout(ctx, musicSideWait)
		defer cancel()
		top = h.artistTopSongs(side, mbid)
	}()
	art, err, answered := WaitFlight[metadata.Artist](ctx, PageFlight(h, ctx, "artist:"+mbid, func(ctx context.Context) (metadata.Artist, error) {
		return h.lib().Artists.Artist(metadata.QuickArtist(ctx, 1500*time.Millisecond), mbid)
	}), by)
	if errors.Is(err, metadata.ErrArtistNotFound) {
		wg.Wait()
		webserver.Fail(w, http.StatusNotFound, "no artist with that id")
		return
	}
	// A slow or failing source never fails the page: it answers with what
	// it has (incomplete) and the lookup goes on, kept for the next ask.
	incomplete := !answered || err != nil
	if incomplete {
		art = metadata.Artist{MBID: mbid, Name: h.knownName("artist", mbid)}
	}
	var dz metadata.DeezerProfile
	var bio *metadata.WikiSummary
	var store []metadata.MusicStoreAlbum
	var storeGap *PageGap
	side, cancel := context.WithTimeout(ctx, max(time.Until(by), musicSideFloor))
	defer cancel()
	if h.MusicDiscography != nil && len(art.Albums) == 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := h.MusicDiscography(side, mbid)
			if err == nil {
				store = d.Albums
			} else {
				storeGap = gapOf(err)
			}
		}()
	}
	wg.Add(2)
	go func() { defer wg.Done(); dz = h.deezerProfile(side, mbid, art) }()
	go func() { defer wg.Done(); bio = h.artistBio(side, mbid, art.Name) }()
	wg.Wait()
	albums, t, err := h.pageAlbums(ctx, r, art, dz)
	if err != nil {
		webserver.Internal(w, "api", err)
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
	h.prefetchTracklists(ctx, firstAlbums(v))
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
	h.rememberMusicNames(songNames)
	var names []nameKey
	for _, l := range links {
		names = append(names, nameKey{kind: "artist", mbid: l.MBID, name: l.Name})
		v.Related = append(v.Related, PageArtist{MBID: l.MBID, Name: l.Name, Image: artPath("artist", l.MBID), Note: l.Type})
	}
	h.rememberMusicNames(names)
	h.warmMusicArt(v)
	musicPageCaching(w, v.Incomplete)
	h.WriteMusicJSON(w, r, v)
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
func (h *Handlers) prefetchTracklists(ctx context.Context, ids []string) {
	if len(ids) == 0 || h.lib() == nil || h.lib().Artists == nil {
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
				fl := PageFlight(h, base, "pressings:"+id, func(ctx context.Context) ([]metadata.Pressing, error) {
					return h.lib().Artists.Pressings(ctx, id)
				})
				<-fl.Done()
			}()
		}
		wg.Wait()
	}()
}

const (
	// musicSideWait caps each optional part of a page (tags, top songs).
	// A part kept answers at once; one not kept goes on behind the answer.
	musicSideWait = 800 * time.Millisecond
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
func (h *Handlers) warmMusicArt(v ArtistPageView) {
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
	h.warmArtPaths(all...)
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
	if len(keys) == 0 || h.lib() == nil || h.lib().Artwork == nil {
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
				h.musicTrendArtFile(ctx, kind, mbid, "500")
			}()
		}
		wg.Wait()
	}()
}
