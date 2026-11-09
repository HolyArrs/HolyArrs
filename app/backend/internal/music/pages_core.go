package music

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

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
func (h *Handlers) knownName(kind, mbid string) string {
	c := &h.music
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.names[kind+"|"+mbid].Name
}

// albumArtistOf names an album's artist when the link did not: from the
// names Explore kept, else from the music source.
func (h *Handlers) albumArtistOf(ctx context.Context, mbid string) string {
	c := &h.music
	c.mu.Lock()
	id := strings.ToLower(c.names["album|"+mbid].ArtistMBID)
	c.mu.Unlock()
	if metadata.ValidMBID(id) {
		return id
	}
	if f, ok := h.lib().Artists.(metadata.AlbumArtistFinder); ok {
		if id, err := f.AlbumArtist(ctx, mbid); err == nil {
			return id
		}
	}
	return ""
}

// albumWithRealArtist answers the album page again for the artist the album
// itself names, when the link's artist is wrong. False when there is no other artist.
func (h *Handlers) albumWithRealArtist(ctx context.Context, w http.ResponseWriter, r *http.Request, mbid, artist string) bool {
	real := h.albumArtistOf(ctx, mbid)
	if real == "" || real == artist {
		return false
	}
	q := r.URL.Query()
	q.Set("artist", real)
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	r2.URL = &u
	h.albumPage(w, r2)
	return true
}

// musicPageGate checks the person, the id and that music is on.
func (h *Handlers) musicPageGate(w http.ResponseWriter, r *http.Request) (string, bool) {
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if h.KidMax(r) != "" {
		webserver.Fail(w, http.StatusForbidden, aboveLimit)
		return "", false
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	if !metadata.ValidMBID(mbid) {
		webserver.Fail(w, http.StatusBadRequest, "mbid must be a MusicBrainz id")
		return "", false
	}
	return mbid, h.musicPageOpen(w)
}

func (h *Handlers) artistCore(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	ctx := metadata.MemberLookup(r.Context())
	bioBy := time.After(coreBioWait)
	t, err := h.libraryArtist(ctx, mbid)
	if err != nil {
		webserver.Internal(w, "api", err)
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
		v.Name = h.knownName("artist", mbid)
	}
	// The bio needs only the name: start it at once when the name is known.
	bioDone := make(chan *metadata.WikiSummary, 1)
	startBio := func(name string) {
		go func() {
			b, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
			defer cancel()
			bioDone <- h.artistBio(b, mbid, name)
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
	art, err := h.lib().Artists.Artist(quick, mbid)
	switch {
	case errors.Is(err, metadata.ErrArtistNotFound):
		webserver.Fail(w, http.StatusNotFound, "no artist with that id")
		return
	case err != nil && v.Name == "":
		webserver.Fail(w, http.StatusBadGateway, "the music source did not answer; try again shortly")
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
			h.rememberMusicNames(names)
			h.warmArtPaths(imgs...)
		}
	}
	select {
	case v.Bio = <-bioDone:
	case <-bioBy:
	}
	h.rememberMusicNames([]nameKey{{kind: "artist", mbid: mbid, name: v.Name}})
	w.Header().Set("Cache-Control", "private, max-age=60")
	h.WriteMusicJSON(w, r, v)
}

// artistSimilar answers GET /api/v1/music/artists/{mbid}?part=similar[&n=14].
func (h *Handlers) artistSimilar(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	n := 18
	if r.URL.Query().Get("n") == "14" {
		n = 14
	}
	list := h.similarArtists(metadata.MemberLookup(r.Context()), mbid, n)
	h.warmMusicArt(ArtistPageView{Similar: list})
	if len(list) == 0 && h.similarPending(mbid) {
		// The server is still getting the list: the page asks again.
		w.Header().Set("Cache-Control", "no-store")
		h.WriteMusicJSON(w, r, map[string]any{"similar": list, "incomplete": true})
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	h.WriteMusicJSON(w, r, map[string]any{"similar": list})
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
func (h *Handlers) coreMissed(mbid, artist string) bool {
	c := &h.music
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.coreMiss[mbid+"|"+artist]
	return ok && time.Since(at) < albumCoreMiss
}

func (h *Handlers) rememberCoreMiss(mbid, artist string) {
	c := &h.music
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.coreMiss == nil || len(c.coreMiss) > 5000 {
		c.coreMiss = map[string]time.Time{}
	}
	c.coreMiss[mbid+"|"+artist] = time.Now()
}

func (h *Handlers) albumCore(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if !metadata.ValidMBID(artist) {
		webserver.Fail(w, http.StatusBadRequest, "send the album's and its artist's MusicBrainz ids (?artist=)")
		return
	}
	ctx := metadata.MemberLookup(r.Context())
	if h.coreMissed(mbid, artist) {
		webserver.Fail(w, http.StatusNotFound, "this album is not kept yet; the full page answers")
		return
	}
	v := AlbumCoreView{MBID: mbid, Image: artPath("album", mbid), SecondaryTypes: []string{}, Artist: PageArtist{MBID: artist, Image: artPath("artist", artist)}}
	if t, err := h.libraryArtist(ctx, artist); err == nil && t != nil {
		v.Artist.Name = t.Name
		if img := artistImage(*t); img != "" {
			v.Artist.Image = img
		}
	}
	// The names Explore and the artist page kept answer at once; the
	// metadata server adds the kind and date when it answers quickly, and
	// a lookup past that goes on behind the answer (the next ask joins it).
	art, err, answered := WaitFlight[metadata.Artist](ctx, PageFlight(h, ctx, "core-artist:"+artist, func(ctx context.Context) (metadata.Artist, error) {
		return h.lib().Artists.Artist(metadata.QuickArtist(ctx, 0), artist)
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
		v.Name = h.knownName("album", mbid)
	}
	if v.Artist.Name == "" {
		v.Artist.Name = h.knownName("artist", artist)
	}
	if v.Name == "" {
		// Not in the kept list (a long discography): the full page answers.
		// Remembered a minute, so a page listing it does not ask each load.
		h.rememberCoreMiss(mbid, artist)
		webserver.Fail(w, http.StatusNotFound, "this album is not kept yet; the full page answers")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	h.WriteMusicJSON(w, r, v)
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
func (h *Handlers) artistMore(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(metadata.MemberLookup(r.Context()), 4*time.Second)
	defer cancel()
	essential, appears := []string{}, []PageCredit{}
	if d := h.musicArtistDataOf(ctx, []string{mbid}, time.Now())[mbid]; d != nil {
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
		h.warmCredits(appears)
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
		h.warmCredits(warm)
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	h.WriteMusicJSON(w, r, map[string]any{"essential": essential, "appears_on": appears})
}

// warmCredits lets the picture route serve these albums (their names and
// artists feed the cover chain's name search) and fetches their covers
// into the artwork cache in the background, so the shelf arrives without
// placeholders. Without the names the picture route has nothing to ask.
func (h *Handlers) warmCredits(list []PageCredit) {
	if len(list) == 0 {
		return
	}
	names := make([]nameKey, 0, len(list))
	items := make([]metadata.MusicTrendItem, 0, len(list))
	for _, c := range list {
		names = append(names, nameKey{kind: "album", mbid: c.MBID, name: c.Name, artist: c.ArtistName})
		items = append(items, metadata.MusicTrendItem{MBID: c.MBID, Name: c.Name, Title: c.Name, Artist: c.ArtistName})
	}
	h.rememberMusicNames(names)
	if !h.PartIsOn(parts.Music) || h.lib() == nil || h.lib().Artwork == nil {
		return
	}
	h.music.prefetching.Add(1)
	go func() {
		defer h.music.prefetching.Done()
		h.prefetchMusicArt("album", items)
	}()
}

// albumNearFresh is how long an album's "more like this" list (an empty
// one too) is answered before it is asked again behind the answer.
const albumNearFresh = 6 * time.Hour

// albumNearOf is the best album of each artist similar to the album's
// artist (the album itself left out), waiting on the similar list as long
// as ctx allows.
func (h *Handlers) albumNearOf(ctx context.Context, mbid, artist string) []PageCredit {
	near := []PageCredit{}
	wait := musicSideWait
	if d, ok := ctx.Deadline(); ok {
		wait = max(time.Until(d)-time.Second, musicSideWait)
	}
	sim := h.similarArtistsWait(ctx, artist, 14, wait)
	ids := make([]string, 0, len(sim))
	for _, p := range sim {
		ids = append(ids, p.MBID)
	}
	data := h.musicArtistDataOf(ctx, ids, time.Now())
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
func (h *Handlers) albumNear(w http.ResponseWriter, r *http.Request) {
	mbid, ok := h.musicPageGate(w, r)
	if !ok {
		return
	}
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if !metadata.ValidMBID(artist) {
		webserver.Fail(w, http.StatusBadRequest, "send the album's and its artist's MusicBrainz ids (?artist=)")
		return
	}
	ctx, cancel := context.WithTimeout(metadata.MemberLookup(r.Context()), 2*musicSideWait)
	defer cancel()
	// Kept (an empty list too) and answered at once; refreshed in the
	// background after albumNearFresh. A cold lookup goes on behind the
	// answer and is kept for the next visit.
	near, _ := SideGet(h, ctx, "near", mbid+"|"+artist, albumNearFresh, func(ctx context.Context) ([]PageCredit, bool, error) {
		return h.albumNearOf(ctx, mbid, artist), true, nil
	})
	if near == nil {
		near = []PageCredit{}
	}
	h.warmCredits(near)
	w.Header().Set("Cache-Control", "private, max-age=300")
	h.WriteMusicJSON(w, r, map[string]any{"near": near})
}
