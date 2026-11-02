package api

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// fakeImages maps "kind/mbid" to a stored file.
type fakeImages map[string]string

func (f fakeImages) Served(_ context.Context, kind, mbid, want string) (string, imgstore.Record, bool, bool) {
	p, ok := f[kind+"/"+mbid]
	return p, imgstore.Record{Kind: kind, ID: mbid, Version: "abc", Sizes: []string{"500"}}, true, ok
}

func TestImageRoute(t *testing.T) {
	file := filepath.Join(t.TempDir(), "500.webp")
	os.WriteFile(file, []byte("jpegbytes"), 0o644)
	var wanted []string
	s, _, calls := setup(Options{Burst: 100, Images: &Images{Store: fakeImages{"album/" + queuedArtist: file}, Want: func(k, m string) { wanted = append(wanted, k+":"+m) }}})
	w := req(s, "/v1/img/album/"+queuedArtist+"/500.webp?v=abc", "")
	if w.Code != 200 || w.Body.String() != "jpegbytes" || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || w.Header().Get("ETag") == "" {
		t.Fatalf("stored %d %v", w.Code, w.Header())
	}
	if w := req(s, "/v1/img/album/"+queuedArtist+"/500.webp", ""); w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=600, s-maxage=3600" {
		t.Fatalf("unversioned %v", w.Header())
	}
	other := "c1392450-e666-3926-a536-22c65f834433"
	w = req(s, "/v1/img/album/"+other+"/250.jpg", "")
	if w.Code != 302 || !strings.HasSuffix(w.Header().Get("Location"), "/release-group/"+other+"/front-250") || w.Header().Get("Cache-Control") != "public, max-age=60, s-maxage=60" {
		t.Fatalf("redirect %d %v", w.Code, w.Header())
	}
	w = req(s, "/v1/img/artist/"+other+"/1200.jpg", "")
	if w.Code != 200 || w.Header().Get("X-Metadata-Placeholder") != "true" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("placeholder %d %v", w.Code, w.Header())
	}
	if len(wanted) != 2 {
		t.Fatalf("jobs %v", wanted)
	}
	for _, p := range []string{"album/" + other + "/300.jpg", "song/" + other + "/500.jpg", "album/nope/500.jpg", "album/" + other + "/500.jpg?x=1"} {
		if w := req(s, "/v1/img/"+p, ""); w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s accepted %d", p, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("image route reached a provider")
	}
}

type liveImages struct {
	stored bool
	path   string
}

func (l *liveImages) Served(_ context.Context, kind, mbid, want string) (string, imgstore.Record, bool, bool) {
	if l.stored {
		return l.path, imgstore.Record{Version: "abc"}, true, true
	}
	return "", imgstore.Record{}, false, false
}

func TestColdArtistImageFetchesNow(t *testing.T) {
	file := filepath.Join(t.TempDir(), "500.webp")
	os.WriteFile(file, []byte("jpegbytes"), 0o644)
	st := &liveImages{path: file}
	var nowKinds []string
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: st, Now: func(_ context.Context, k, m string) bool {
		nowKinds = append(nowKinds, k)
		st.stored = true
		return true
	}}})
	other := "c1392450-e666-3926-a536-22c65f834433"
	if w := req(s, "/v1/img/artist/"+other+"/500.webp", ""); w.Code != 200 || w.Body.String() != "jpegbytes" {
		t.Fatalf("cold artist %d %v", w.Code, w.Header())
	}
	// An album, wide background or logo is fetched at once too.
	st.stored = false
	if w := req(s, "/v1/img/album/"+other+"/500.webp", ""); w.Code != 200 || len(nowKinds) != 2 || nowKinds[1] != "album" {
		t.Fatalf("album %d now %v", w.Code, nowKinds)
	}
}

// An artist's wide background and logo are served from the store as
// /v1/img/wide/{mbid}/{500|960|1200|1920}.{webp|jpg} and
// /v1/img/logo/{mbid}/{500|1200}.png;
// one not stored yet is a 404 kept nowhere, and its job is queued.
func TestWideAndLogoRoutes(t *testing.T) {
	dir := t.TempDir()
	wide, logo := filepath.Join(dir, "1920.webp"), filepath.Join(dir, "1200.png")
	os.WriteFile(wide, []byte("widebytes"), 0o644)
	os.WriteFile(logo, []byte("pngbytes"), 0o644)
	var wanted []string
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: fakeImages{"wide/" + queuedArtist: wide, "logo/" + queuedArtist: logo}, Want: func(k, m string) { wanted = append(wanted, k+":"+m) }}})
	if w := req(s, "/v1/img/wide/"+queuedArtist+"/1920.webp", ""); w.Code != 200 || w.Body.String() != "widebytes" || w.Header().Get("Content-Type") != "image/webp" {
		t.Fatalf("wide %d %v", w.Code, w.Header())
	}
	if w := req(s, "/v1/img/logo/"+queuedArtist+"/1200.png", ""); w.Code != 200 || w.Body.String() != "pngbytes" || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("logo %d %v", w.Code, w.Header())
	}
	other := "c1392450-e666-3926-a536-22c65f834433"
	if w := req(s, "/v1/img/logo/"+other+"/500.png", ""); w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing logo %d %v", w.Code, w.Header())
	}
	if w := req(s, "/v1/img/wide/"+queuedArtist+"/960.webp", ""); w.Code != 200 {
		t.Fatalf("wide 960 %d", w.Code)
	}
	for _, bad := range []string{"/v1/img/logo/" + queuedArtist + "/1200.jpg", "/v1/img/wide/" + queuedArtist + "/250.jpg", "/v1/img/logo/" + queuedArtist + "/1920.png"} {
		if w := req(s, bad, ""); w.Code != 400 {
			t.Fatalf("%s: %d", bad, w.Code)
		}
	}
	if len(wanted) != 1 || wanted[0] != "logo:"+other {
		t.Fatalf("jobs %v", wanted)
	}
}

// A background ask (X-Fetch: ahead: an install's preload or warm, nobody
// waiting) never becomes a P0 or P1 job: no inline fetch, and the missing
// picture is queued through WantAhead (P2 or lower), not Want.
func TestBackgroundImageAskIsNotUrgent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "500.jpg")
	os.WriteFile(file, []byte("jpegbytes"), 0o644)
	st := &liveImages{path: file}
	var now, want, ahead []string
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: st,
		Now:       func(_ context.Context, k, m string) bool { now = append(now, k); return false },
		Want:      func(k, m string) { want = append(want, k) },
		WantAhead: func(k, m string) { ahead = append(ahead, k) }}})
	other := "c1392450-e666-3926-a536-22c65f834433"
	for _, kind := range []string{"artist/" + other + "/500.jpg", "album/" + other + "/500.jpg", "wide/" + other + "/1920.jpg"} {
		r := httptest.NewRequest("GET", "/v1/img/"+kind, nil)
		r.Header.Set("X-Fetch", "ahead")
		s.ServeHTTP(httptest.NewRecorder(), r)
	}
	if len(now) != 0 || len(want) != 0 || len(ahead) != 3 {
		t.Fatalf("background asks: now %v want %v ahead %v", now, want, ahead)
	}
	// Someone waiting (no marker) keeps the urgent path.
	req(s, "/v1/img/artist/"+other+"/500.jpg", "")
	if len(now) != 1 || len(want) == 0 {
		t.Fatalf("waiting ask: now %v want %v", now, want)
	}
}

// An ask someone waits on (an open page, no ahead mark) queues the missing
// picture at P0 through WantNow, under the asking install.
func TestOpenPageImageAskIsP0(t *testing.T) {
	st := &liveImages{path: filepath.Join(t.TempDir(), "x.jpg")}
	var now []string
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: st,
		Want:    func(k, m string) { t.Errorf("Want used for an open page: %s", k) },
		WantNow: func(_ context.Context, k, m string) { now = append(now, k) }}})
	other := "c1392450-e666-3926-a536-22c65f834433"
	req(s, "/v1/img/album/"+other+"/500.jpg", "")
	req(s, "/v1/img/wide/"+other+"/1920.jpg", "")
	if len(now) != 2 {
		t.Fatalf("P0 asks %v", now)
	}
}

// A picture stored as WebP is served as WebP; an older build's .jpg ask is
// made from it on the fly (a real JPEG, never stored).
func TestStoredWebPAndJPEGOnTheFly(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 200, 40, 40, 255
	}
	b, err := imgstore.EncodeWebP(img)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "500.webp")
	os.WriteFile(file, b, 0o644)
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: fakeImages{"album/" + queuedArtist: file}}})
	base := "/v1/img/album/" + queuedArtist + "/"
	w := req(s, base+"500.webp?v=abc", "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/webp" || !bytes.Equal(w.Body.Bytes(), b) || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("webp %d %v", w.Code, w.Header())
	}
	w = req(s, base+"250.jpg?v=abc", "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("jpeg %d %v", w.Code, w.Header())
	}
	got, err := jpeg.Decode(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	if r, _, _, _ := got.At(10, 10).RGBA(); r>>8 < 170 || color.GrayModel.Convert(got.At(10, 10)).(color.Gray).Y > 120 {
		t.Fatalf("colours lost: %v", got.At(10, 10))
	}
}

// big asked before it is made: the card size stands in for a minute and
// the big size is queued.
type bigPending struct{ path string }

func (b bigPending) Served(_ context.Context, kind, mbid, want string) (string, imgstore.Record, bool, bool) {
	return b.path, imgstore.Record{Version: "abc", Sizes: []string{"500"}, BigURL: "x"}, want != "1200", true
}

func TestBigNotMadeYetStandsIn(t *testing.T) {
	small, err := imgstore.EncodeWebP(image.NewNRGBA(image.Rect(0, 0, 8, 8)))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "500.webp")
	os.WriteFile(file, small, 0o644)
	var bigs []string
	s, _, _ := setup(Options{Burst: 100, Images: &Images{Store: bigPending{file}, WantBig: func(k, m string) { bigs = append(bigs, k) }}})
	w := req(s, "/v1/img/artist/"+queuedArtist+"/1200.webp?v=abc", "")
	if w.Code != 200 || w.Header().Get("Cache-Control") == "public, max-age=31536000, immutable" || len(bigs) != 1 {
		t.Fatalf("stand-in kept short and big queued: %d %v %v", w.Code, w.Header(), bigs)
	}
	if w := req(s, "/v1/img/artist/"+queuedArtist+"/500.webp?v=abc", ""); w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || len(bigs) != 1 {
		t.Fatalf("card size is final: %v", w.Header())
	}
	// A stand-in chosen by Accept stays browser-only: never one edge copy
	// for every format.
	for _, accept := range []string{"image/webp,*/*", "image/jpeg"} {
		w := reqAccept(s, "/v1/img/artist/"+queuedArtist+"/1200?v=abc", accept)
		if w.Code != 200 || w.Header().Get("Vary") != "Accept" || w.Header().Get("Cache-Control") != "private, max-age=60" {
			t.Fatalf("partial and negotiated (%s): %d %v", accept, w.Code, w.Header())
		}
	}
}

func reqAccept(s *Server, path, accept string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Accept", accept)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
