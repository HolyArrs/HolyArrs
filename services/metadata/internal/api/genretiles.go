package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // fanart.tv and Deezer pictures may be PNG
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Genre tile art (plan 2026-10-22, Phase 2): one artist photo per genre,
// cropped to 16:9, turned grey and kept as a small JPEG (at most 40 KB).
// The app paints the genre hue and lays this picture on it at .55
// opacity, so the duotone needs no blend modes. The photo is fanart.tv's
// artist background (key on the server) or Deezer's artist picture; with
// neither the genre keeps no art and the app shows the flat hue.

const (
	tileW, tileH = 480, 270
	tileMaxBytes = 40 << 10
	tileTTL      = 7 * 24 * time.Hour
	tileMissTTL  = time.Hour
	// tilePartialTTL is how long a slideshow with fewer than tilePhotos photos stands.
	tilePartialTTL = 6 * time.Hour
	tileWorkers    = 2
	// tilePhotos is the slideshow size; tileCandidates the artists tried.
	tilePhotos     = 4
	tileCandidates = 30
	// tileMax caps the rendered tiles kept; the oldest goes first.
	tileMax = 256
)

// GenreTiles renders and keeps genre tile pictures. The zero value with
// no sources renders nothing.
type GenreTiles struct {
	// Fanart answers artist art JSON (kind "music", id an artist MBID).
	Fanart ArtSource
	// DeezerURL is the Deezer API root; empty skips Deezer.
	DeezerURL string
	UserAgent string
	HTTP      *http.Client
	Now       func() time.Time
	// Store keeps chosen tiles across restarts; nil keeps them in memory only.
	Store *TileStore

	mu    sync.Mutex
	tiles map[string]genreSet
	busy  map[string]bool
	// used maps an artist to the genre whose tile shows them, so two
	// genres never share a photo.
	used  map[string]string
	slots chan struct{}
}

// genreTile is one artist photo of a genre's slideshow.
type genreTile struct {
	jpeg   []byte
	etag   string
	artist string
	mbid   string
	// link is the source picture the tile was rendered from.
	link string
}

// genreSet is a genre's slideshow: up to tilePhotos artist photos, no covers.
type genreSet struct {
	photos []genreTile
	at     time.Time
}

// TileArtist is the artist a genre tile is drawn from.
type TileArtist struct {
	MBID, Name string
}

func (g *GenreTiles) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// On reports whether any picture source is set.
func (g *GenreTiles) On() bool {
	if g == nil {
		return false
	}
	return g.fanartOn() || g.DeezerURL != ""
}

func (g *GenreTiles) fanartOn() bool {
	return g.Fanart != nil && !strings.HasPrefix(string(g.Fanart.Status().State), "disabled")
}

// Ready answers the first photo's version tag ("" when there is none yet,
// or the last try found no picture).
func (g *GenreTiles) Ready(slug string) string {
	if v := g.Versions(slug); len(v) > 0 {
		return v[0]
	}
	return ""
}

// Versions answers the version tag of each of a genre's photos, in order.
func (g *GenreTiles) Versions(slug string) []string {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.lookupLocked(slug)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(s.photos))
	for _, p := range s.photos {
		out = append(out, p.etag)
	}
	return out
}

// Get answers photo idx of a genre.
func (g *GenreTiles) Get(slug string, idx int) ([]byte, string, bool) {
	if g == nil {
		return nil, "", false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.lookupLocked(slug)
	if !ok || idx < 0 || idx >= len(s.photos) {
		return nil, "", false
	}
	return s.photos[idx].jpeg, s.photos[idx].etag, true
}

// Want renders slug's photos in the background when they are missing or
// old; pick lists the genre's artists, best first, lazily (it may read
// charts). Artists shown by another genre are skipped.
func (g *GenreTiles) Want(slug string, pick func() []TileArtist) {
	if !g.On() {
		return
	}
	now := g.now()
	g.mu.Lock()
	g.initLocked()
	t, ok := g.lookupLocked(slug)
	ttl := tileTTL
	if ok && len(t.photos) == 0 {
		ttl = tileMissTTL
	} else if ok && len(t.photos) < tilePhotos {
		ttl = tilePartialTTL
	}
	if g.busy[slug] || (ok && now.Sub(t.at) < ttl) {
		g.mu.Unlock()
		return
	}
	if len(g.busy) >= tileMax {
		g.mu.Unlock()
		return
	}
	if !ok && len(g.tiles) >= tileMax {
		g.evictOldest()
	}
	prev, hadPrev := t, ok
	g.busy[slug] = true
	slots := g.slots
	g.mu.Unlock()
	go func() {
		slots <- struct{}{}
		defer func() { <-slots }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		// A picked set whose pictures all still answer HEAD is kept; one
		// with a gone picture is picked again.
		if hadPrev && !short(prev) {
			alive := true
			for _, p := range prev.photos {
				if p.link == "" || !g.headOK(ctx, p.link) {
					alive = false
					break
				}
			}
			if alive {
				g.mu.Lock()
				defer g.mu.Unlock()
				delete(g.busy, slug)
				prev.at = g.now()
				g.tiles[slug] = prev
				g.save(slug, prev)
				return
			}
		}
		var photos []genreTile
		mine := map[string]bool{}
		artists := pick()
		if len(artists) > tileCandidates {
			artists = artists[:tileCandidates]
		}
		// Two passes: fanart.tv's artist photos for every artist first,
		// then Deezer's artist pictures. Only artist pictures are used,
		// never an album or release cover.
		for pass := 0; pass < 2 && len(photos) < tilePhotos; pass++ {
			for _, a := range artists {
				if len(photos) >= tilePhotos {
					break
				}
				if !photoArtist(a.Name) {
					continue
				}
				k := strings.ToLower(a.MBID + "|" + a.Name)
				if mine[k] {
					continue
				}
				g.mu.Lock()
				other, taken := g.used[k]
				if !taken || other == slug {
					g.used[k] = slug
				}
				g.mu.Unlock()
				if taken && other != slug {
					continue
				}
				if p, link, err := g.render(ctx, a, pass == 1); err == nil {
					mine[k] = true
					sum := sha256.Sum256(p)
					photos = append(photos, genreTile{jpeg: p, etag: hex.EncodeToString(sum[:6]), artist: k, mbid: a.MBID, link: link})
					continue
				}
				g.mu.Lock()
				if g.used[k] == slug && !prevHas(prev, k) {
					delete(g.used, k)
				}
				g.mu.Unlock()
			}
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.busy, slug)
		ns := genreSet{photos: photos, at: g.now()}
		if len(photos) > 0 {
			for _, p := range prev.photos {
				if !mine[p.artist] && g.used[p.artist] == slug {
					delete(g.used, p.artist)
				}
			}
		}
		if hadPrev && len(prev.photos) > len(photos) && !badName(prev) {
			// Keep the older photos through a failed or poorer refresh.
			ns = prev
			ns.at = g.now()
		}
		g.tiles[slug] = ns
		g.save(slug, ns)
	}()
}

// short reports a slideshow that is not full or shows a name that is no longer
// taken for a photo artist; it is picked again right after a start.
func short(s genreSet) bool {
	if len(s.photos) < tilePhotos {
		return true
	}
	return badName(s)
}

// badName reports a slideshow that shows a name no longer taken for a photo artist.
func badName(s genreSet) bool {
	for _, p := range s.photos {
		_, name, _ := strings.Cut(p.artist, "|")
		if !photoArtist(name) {
			return true
		}
	}
	return false
}

func prevHas(s genreSet, artist string) bool {
	for _, p := range s.photos {
		if p.artist == artist {
			return true
		}
	}
	return false
}

// evictOldest drops the oldest genre and frees its artists; g.mu is held.
func (g *GenreTiles) evictOldest() {
	var oldest string
	var at time.Time
	for k, t := range g.tiles {
		if oldest == "" || t.at.Before(at) {
			oldest, at = k, t.at
		}
	}
	if t, ok := g.tiles[oldest]; ok {
		for _, p := range t.photos {
			if p.artist != "" && g.used[p.artist] == oldest {
				delete(g.used, p.artist)
			}
		}
		delete(g.tiles, oldest)
	}
}

var errNoPicture = errors.New("genre tile: no picture")

// photoArtist reports whether an artist name can stand for a person or a
// band with a photo: compilation credits ("Various Artists", a cast or a
// soundtrack) have only covers and word clouds.
func photoArtist(name string) bool {
	n := strings.ToLower(name)
	for _, w := range []string{"various", "soundtrack", "original cast", "cast recording", "motion picture", "compilation", "lullaby", "academy", "karaoke", "tribute", "nursery"} {
		if strings.Contains(n, w) {
			return false
		}
	}
	return strings.TrimSpace(n) != ""
}

func (g *GenreTiles) render(ctx context.Context, a TileArtist, deezer bool) ([]byte, string, error) {
	for _, link := range g.pictures(ctx, a, deezer) {
		img, err := g.fetchImage(ctx, link)
		if err != nil {
			continue
		}
		if out, err := TileJPEG(img); err == nil {
			return out, link, nil
		}
	}
	return nil, "", errNoPicture
}

// initLocked makes the maps; g.mu is held.
func (g *GenreTiles) initLocked() {
	if g.tiles == nil {
		g.tiles, g.busy, g.used, g.slots = map[string]genreSet{}, map[string]bool{}, map[string]string{}, make(chan struct{}, tileWorkers)
	}
}

// lookupLocked answers slug's photos from memory, else from the store (a
// genre dropped by the cap, or kept from before a restart); g.mu is held.
func (g *GenreTiles) lookupLocked(slug string) (genreSet, bool) {
	if t, ok := g.tiles[slug]; ok {
		return t, true
	}
	if g.Store == nil {
		return genreSet{}, false
	}
	sts := g.Store.Get(slug)
	if len(sts) == 0 {
		return genreSet{}, false
	}
	g.initLocked()
	if len(g.tiles) >= tileMax {
		g.evictOldest()
	}
	return g.adoptLocked(slug, sts), true
}

// adoptLocked puts stored photos in memory and claims their artists.
func (g *GenreTiles) adoptLocked(slug string, sts []storedTile) genreSet {
	s := genreSet{}
	for _, st := range sts {
		s.photos = append(s.photos, genreTile{jpeg: st.JPEG, etag: st.ETag, artist: st.Artist, mbid: st.MBID, link: st.ImageURL})
		if st.PickedAt.After(s.at) {
			s.at = st.PickedAt
		}
		if st.Artist != "" {
			if _, taken := g.used[st.Artist]; !taken {
				g.used[st.Artist] = slug
			}
		}
	}
	// A short slideshow is looked at again at once after a start.
	if short(s) {
		s.at = time.Time{}
	}
	g.tiles[slug] = s
	return s
}

// Restore loads the stored genres (up to the cap) so the first genre
// answer after a start already links its art.
func (g *GenreTiles) Restore() {
	if g == nil || g.Store == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initLocked()
	for _, slug := range g.Store.Slugs(tileMax) {
		if _, ok := g.tiles[slug]; !ok && len(g.tiles) < tileMax {
			g.adoptLocked(slug, g.Store.Get(slug))
		}
	}
}

// save writes a picked set to the store; g.mu is held.
func (g *GenreTiles) save(slug string, s genreSet) {
	if g.Store == nil || len(s.photos) == 0 {
		return
	}
	var sts []storedTile
	for i, p := range s.photos {
		if p.link == "" {
			return
		}
		sts = append(sts, storedTile{Slug: slug, Idx: i, Artist: p.artist, MBID: p.mbid, ImageURL: p.link, JPEG: p.jpeg, ETag: p.etag, PickedAt: s.at})
	}
	_ = g.Store.PutSet(slug, sts)
}

// headOK reports whether link still answers a HEAD request with success.
func (g *GenreTiles) headOK(ctx context.Context, link string) bool {
	if u, err := url.Parse(link); err != nil || u.Scheme != "https" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, link, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", g.UserAgent)
	resp, err := g.client().Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// pictures lists candidate photo links, best first.
// With deezer it lists only Deezer's picture; without, only fanart.tv's.
func (g *GenreTiles) pictures(ctx context.Context, a TileArtist, deezer bool) []string {
	var out []string
	if !deezer && g.fanartOn() && a.MBID != "" {
		if body, _, err := g.Fanart.Art(ctx, "music", strings.ToLower(a.MBID)); err == nil {
			var art struct {
				Backgrounds []string `json:"backgrounds"`
				Thumb       string   `json:"thumb"`
			}
			if json.Unmarshal(body, &art) == nil {
				for _, b := range art.Backgrounds {
					// fanart.tv's preview size is plenty for a 480 px tile.
					if p := strings.Replace(b, "/fanart/", "/preview/", 1); p != b {
						out = append(out, p)
					}
					out = append(out, b)
					break
				}
				if art.Thumb != "" {
					out = append(out, art.Thumb)
				}
			}
		}
	}
	if deezer && g.DeezerURL != "" && a.Name != "" {
		if p := g.deezerPicture(ctx, a.Name); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (g *GenreTiles) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (g *GenreTiles) deezerPicture(ctx context.Context, name string) string {
	u := strings.TrimRight(g.DeezerURL, "/") + "/search/artist?" + url.Values{"q": {name}, "limit": {"5"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", g.UserAgent)
	resp, err := g.client().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Data []struct {
			Name    string `json:"name"`
			Picture string `json:"picture_xl"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return ""
	}
	for _, d := range body.Data {
		// Deezer's placeholder pictures have an empty hash in the path.
		if strings.EqualFold(strings.TrimSpace(d.Name), strings.TrimSpace(name)) && d.Picture != "" && !strings.Contains(d.Picture, "/artist//") {
			return d.Picture
		}
	}
	return ""
}

func (g *GenreTiles) fetchImage(ctx context.Context, link string) (image.Image, error) {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" {
		return nil, errNoPicture
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", g.UserAgent)
	resp, err := g.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errNoPicture
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, 8<<20))
	return img, err
}

// TileJPEG crops src to 16:9 around its centre (a little above, where
// faces sit), scales it to 480×270 by area averaging, turns it grey and
// encodes it at most 40 KB.
func TileJPEG(src image.Image) ([]byte, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 16 || h < 9 {
		return nil, errNoPicture
	}
	cw, ch := w, w*9/16
	if ch > h {
		cw, ch = h*16/9, h
	}
	x0 := b.Min.X + (w-cw)/2
	y0 := b.Min.Y + (h-ch)/3
	dst := image.NewGray(image.Rect(0, 0, tileW, tileH))
	for y := 0; y < tileH; y++ {
		sy0, sy1 := y0+y*ch/tileH, y0+(y+1)*ch/tileH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < tileW; x++ {
			sx0, sx1 := x0+x*cw/tileW, x0+(x+1)*cw/tileW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var sum, n uint64
			// At most 4×4 samples per pixel keeps big photos quick.
			stepX, stepY := max(1, (sx1-sx0)/4), max(1, (sy1-sy0)/4)
			for sy := sy0; sy < sy1; sy += stepY {
				for sx := sx0; sx < sx1; sx += stepX {
					gr := color.GrayModel.Convert(src.At(sx, sy)).(color.Gray)
					sum += uint64(gr.Y)
					n++
				}
			}
			dst.SetGray(x, y, color.Gray{Y: uint8(sum / n)})
		}
	}
	for q := 72; q >= 30; q -= 8 {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if buf.Len() <= tileMaxBytes {
			return buf.Bytes(), nil
		}
	}
	return nil, errNoPicture
}

// serveMusicGenreTile answers GET /v1/music/genres/tile/{slug}.jpg.
func (s *Server) serveMusicGenreTile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/v1/music/genres/tile/")
	slug, ok := strings.CutSuffix(name, ".jpg")
	// {slug}.{n}.jpg is photo n of the slideshow; {slug}.jpg is the first.
	idx := 0
	if i := strings.LastIndexByte(slug, '.'); ok && i > 0 {
		if n, err := strconv.Atoi(slug[i+1:]); err == nil && n > 0 && n < tilePhotos {
			slug, idx = slug[:i], n
		}
	}
	if !ok || !slugRe.MatchString(slug) || !onlyParams(w, r, "v") {
		if ok && slugRe.MatchString(slug) {
			return
		}
		problem(w, 400, "invalid_query", 0)
		return
	}
	pic, etag, ok := s.opts.GenreTiles.Get(slug, idx)
	if !ok {
		problem(w, 404, "not_found", 0)
		return
	}
	h := w.Header()
	h.Set("ETag", `"`+etag+`"`)
	h.Set("Cache-Control", "private, max-age=604800")
	if r.Header.Get("If-None-Match") == `"`+etag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "image/jpeg")
	w.WriteHeader(200)
	_, _ = w.Write(pic)
}
