package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fakeAudioDB(t *testing.T, calls *atomic.Int32, agent *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if agent != nil {
			agent.Store(r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/7/artist-mb.php" && r.URL.Query().Get("i") == "mb-lab":
			_, _ = w.Write([]byte(`{"artists":[{"strArtist":"Lab Band","strMusicBrainzID":"mb-lab","strArtistThumb":"https://img.test/thumb.jpg",
				"strArtistFanart":"https://img.test/fanart.jpg","strArtistLogo":"https://img.test/logo.png"}]}`))
		case r.URL.Path == "/7/artist-mb.php":
			_, _ = w.Write([]byte(`{"artists":null}`))
		case r.URL.Path == "/7/search.php" && r.URL.Query().Get("s") == "Named Band":
			_, _ = w.Write([]byte(`{"artists":[{"strArtist":"Named Band Tribute","strArtistThumb":"https://img.test/wrong.jpg"},
				{"strArtist":"The Named Band!","strArtistThumb":"https://img.test/named.jpg"}]}`))
		case r.URL.Path == "/7/search.php":
			_, _ = w.Write([]byte(`{"artists":null}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestAudioDBByIDThenName: the MusicBrainz id is asked first, and its
// thumb, fanart and logo come back; with no id match the name search finds
// the artist by a loose name match; the key is in the path; the
// User-Agent names HolyArrs.
func TestAudioDBByIDThenName(t *testing.T) {
	var calls atomic.Int32
	var agent atomic.Value
	srv := fakeAudioDB(t, &calls, &agent)
	a := &AudioDB{BaseURL: srv.URL, Key: func(context.Context) (string, error) { return "7", nil }, Gap: time.Millisecond, UserAgent: "HolyArrs/test"}
	ctx := context.Background()
	p, err := a.Pictures(ctx, "mb-lab", "Lab Band")
	if err != nil || p.Thumb != "https://img.test/thumb.jpg" || p.Fanart != "https://img.test/fanart.jpg" || p.Logo != "https://img.test/logo.png" {
		t.Fatalf("by id %+v %v", p, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("an id match asked by name too: %d calls", calls.Load())
	}
	if ua, _ := agent.Load().(string); !strings.HasPrefix(ua, "HolyArrs/") {
		t.Fatalf("user agent %q", ua)
	}
	p, err = a.Pictures(ctx, "mb-other", "Named Band")
	if err != nil || p.Thumb != "https://img.test/named.jpg" {
		t.Fatalf("by name %+v %v", p, err)
	}
	p, err = a.Pictures(ctx, "", "Nobody")
	if err != nil || p != (AudioDBPictures{}) {
		t.Fatalf("nobody %+v %v", p, err)
	}
	var nilA *AudioDB
	if p, err := nilA.Pictures(ctx, "mb-lab", "Lab Band"); err != nil || p != (AudioDBPictures{}) {
		t.Fatalf("nil %+v %v", p, err)
	}
}

// TestAudioDBNoKeyAsksNothing: HolyArrs ships no key; with none set
// TheAudioDB is not asked at all.
func TestAudioDBNoKeyAsksNothing(t *testing.T) {
	var calls atomic.Int32
	srv := fakeAudioDB(t, &calls, nil)
	for _, a := range []*AudioDB{{BaseURL: srv.URL, Gap: time.Millisecond},
		{BaseURL: srv.URL, Gap: time.Millisecond, Key: func(context.Context) (string, error) { return "  ", nil }}} {
		if p, err := a.Pictures(context.Background(), "mb-lab", "Lab Band"); err != nil || p != (AudioDBPictures{}) || a.On(context.Background()) {
			t.Fatalf("%+v %v", p, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("asked with no key: %d", calls.Load())
	}
}

// TestAudioDBSpacing: calls on the free key are at least the gap apart
// (2 s by default).
func TestAudioDBSpacing(t *testing.T) {
	if (&AudioDB{}).gap() < 2*time.Second {
		t.Fatal("default gap under 2 s")
	}
	var calls atomic.Int32
	srv := fakeAudioDB(t, &calls, nil)
	a := &AudioDB{BaseURL: srv.URL, Key: func(context.Context) (string, error) { return "7", nil }, Gap: 80 * time.Millisecond}
	start := time.Now()
	_, _ = a.Pictures(context.Background(), "a", "")
	_, _ = a.Pictures(context.Background(), "b", "")
	if time.Since(start) < 80*time.Millisecond {
		t.Fatal("calls not spaced")
	}
}

func TestNormalName(t *testing.T) {
	for in, want := range map[string]string{"The Named Band!": "namedband", "AC/DC": "acdc", "  Beyoncé ": "beyonce", "Sigur Rós": "sigurros"} {
		if got := normalName(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// TestAudioDBBacksOffOn429: a 429 waits Retry-After and asks once more.
func TestAudioDBBacksOffOn429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"artists":[{"strArtist":"Lab Band","strArtistThumb":"https://img.test/thumb.jpg"}]}`))
	}))
	t.Cleanup(srv.Close)
	a := &AudioDB{BaseURL: srv.URL, Key: func(context.Context) (string, error) { return "7", nil }, Gap: time.Millisecond}
	start := time.Now()
	p, err := a.Pictures(context.Background(), "mb-lab", "Lab Band")
	if err != nil || p.Thumb != "https://img.test/thumb.jpg" || calls.Load() != 2 {
		t.Fatalf("%+v %v calls=%d", p, err, calls.Load())
	}
	if time.Since(start) < time.Second {
		t.Fatal("did not wait Retry-After")
	}
}
