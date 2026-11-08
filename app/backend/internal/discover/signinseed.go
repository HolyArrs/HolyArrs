package discover

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The sign-in page's first frame on a first-ever visit. index.html for
// /login carries the art list and a tiny mosaic (24 and 64 px wide JPEGs)
// of the first slide's backdrop and poster, so app.html paints the photo
// before any app script runs. Same art as /api/v1/public/signin-art (same
// cache, same Library-art setting); picture data only, nothing else.

const (
	// signinSeedFetchMax caps one downloaded small picture.
	signinSeedFetchMax = 1 << 20
	// signinSeedThumbMax caps one encoded thumbnail (data URI).
	signinSeedThumbMax = 6 << 10
	// signinSeedKeep is how many pictures' thumbnails are kept.
	signinSeedKeep = 16
	// signinSeedRetry is how long a failed thumbnail waits before a new try.
	signinSeedRetry = time.Minute
)

// signinThumb is one picture's tiny copies, as data URIs.
type signinThumb struct {
	T24 string `json:"t24"`
	T64 string `json:"t64"`
}

type signinSeedState struct {
	mu     sync.Mutex
	thumbs map[string]signinThumb // by original picture address
	tried  map[string]time.Time   // failed or running, by address
	// rebuilding is set while a background art build started here runs,
	// so a flood of /login opens starts one build, not one per request.
	rebuilding atomic.Bool
}

// signinSeedJSON is what index.html carries for /login, or nil when the
// art or its thumbnails are not ready yet (then their build starts in the
// background and the page is served without waiting).
func (s *Signin) signinSeedJSON(ctx context.Context) []byte {
	if s.Store == nil {
		return nil
	}
	shots, ok := s.signinShotsCached(ctx)
	if !ok && s.seed.rebuilding.CompareAndSwap(false, true) {
		go func() {
			defer s.seed.rebuilding.Store(false)
			s.signinShots(context.WithoutCancel(ctx))
		}()
	}
	if len(shots) == 0 {
		return nil
	}
	first := shots[0]
	seed := map[string]any{}
	back, ok := s.signinThumbFor(first.Backdrop.URL, 300)
	if !ok {
		return nil
	}
	seed["backdrop"] = back
	if first.Poster != nil {
		if p, ok := s.signinThumbFor(first.Poster.URL, 342); ok {
			seed["poster"] = p
		}
	}
	// json.Marshal escapes <, > and &, so the text is safe inside <script>.
	b, err := json.Marshal(map[string]any{"shots": publicShots(shots), "seed": seed})
	if err != nil {
		return nil
	}
	return b
}

// signinShotsCached is the cached answer without building; ok is false
// when it is missing or stale.
func (s *Signin) signinShotsCached(ctx context.Context) ([]SigninShot, bool) {
	st := &s.art
	lib := s.signinLibraryArt(ctx)
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.cache[lib]
	return c.shots, ok && st.clock().Before(c.until)
}

// signinThumbFor returns the kept thumbnail of an original TMDB picture,
// starting its build when it is not kept yet.
func (s *Signin) signinThumbFor(original string, size int) (signinThumb, bool) {
	ss := &s.seed
	now := s.art.clock()
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if t, ok := ss.thumbs[original]; ok {
		return t, true
	}
	if at, ok := ss.tried[original]; ok && now.Sub(at) < signinSeedRetry {
		return signinThumb{}, false
	}
	if ss.tried == nil {
		ss.tried = map[string]time.Time{}
	}
	ss.tried[original] = now
	go func() {
		t, err := s.makeSigninThumb(original, size)
		ss.mu.Lock()
		defer ss.mu.Unlock()
		if err != nil {
			return // retried after signinSeedRetry
		}
		delete(ss.tried, original)
		if ss.thumbs == nil || len(ss.thumbs) >= signinSeedKeep {
			ss.thumbs = map[string]signinThumb{}
		}
		ss.thumbs[original] = t
	}()
	return signinThumb{}, false
}

// makeSigninThumb downloads TMDB's small size of the picture and shrinks it.
func (s *Signin) makeSigninThumb(original string, size int) (signinThumb, error) {
	file, ok := strings.CutPrefix(original, metadata.ImageBase+"/original/")
	if !ok || file == "" || strings.ContainsAny(file, "/?#\\") {
		return signinThumb{}, errors.New("not a TMDB original")
	}
	url := metadata.ImageBase + "/w" + strconv.Itoa(size) + "/" + file
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fetch := s.Fetch
	if fetch == nil {
		fetch = fetchSmall
	}
	b, err := fetch(ctx, url)
	if err != nil {
		return signinThumb{}, err
	}
	return signinThumbOf(b)
}

func fetchSmall(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("picture answered " + res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, signinSeedFetchMax+1))
	if err != nil {
		return nil, err
	}
	if len(b) > signinSeedFetchMax {
		return nil, errors.New("picture too big")
	}
	return b, nil
}

// signinThumbOf turns a JPEG into its 24 and 64 px wide copies.
func signinThumbOf(b []byte) (signinThumb, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return signinThumb{}, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 2000 || cfg.Height > 3000 {
		return signinThumb{}, errors.New("unexpected picture size")
	}
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return signinThumb{}, err
	}
	var out [2]string
	for i, w := range []int{24, 64} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, shrink(img, w), &jpeg.Options{Quality: 72}); err != nil {
			return signinThumb{}, err
		}
		s := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		if len(s) > signinSeedThumbMax {
			return signinThumb{}, errors.New("thumbnail too big")
		}
		out[i] = s
	}
	return signinThumb{T24: out[0], T64: out[1]}, nil
}

// shrink averages src into a w-wide copy (box filter), keeping its shape.
func shrink(src image.Image, w int) image.Image {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	w = min(w, sw)
	h := max(1, (w*sh+sw/2)/sw)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, b, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, _ := src.At(sb.Min.X+xx, sb.Min.Y+yy).RGBA()
					r, g, b, n = r+uint64(cr), g+uint64(cg), b+uint64(cb), n+1
				}
			}
			dst.Set(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(b / n >> 8), 255})
		}
	}
	return dst
}

// warmSigninSeed starts the first slide's thumbnails, so the next /login
// already carries them.
func (s *Signin) warmSigninSeed(shots []SigninShot) {
	if len(shots) == 0 {
		return
	}
	s.signinThumbFor(shots[0].Backdrop.URL, 300)
	if p := shots[0].Poster; p != nil {
		s.signinThumbFor(p.URL, 342)
	}
}
