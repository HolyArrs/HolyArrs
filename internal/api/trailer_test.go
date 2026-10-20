package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// TestFakeYTDLP is not a test: run as a child process it plays yt-dlp,
// answering with what FAKE_YTDLP says (ok, slow, fail, odd).
func TestFakeYTDLP(t *testing.T) {
	mode := os.Getenv("FAKE_YTDLP")
	if mode == "" {
		return
	}
	args := strings.Join(os.Args, " ")
	if !strings.Contains(args, "watch?v=trailerKey1") || !strings.Contains(args, "height<=720") {
		os.Exit(3)
	}
	exp := time.Now().Add(6 * time.Hour).Unix()
	switch mode {
	case "ok":
		fmt.Printf(`{"url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=22","ext":"mp4","vcodec":"avc1.64001F","acodec":"mp4a.40.2","width":1280,"height":720,"protocol":"https"}`, exp)
	case "odd":
		fmt.Print(`{"url":"https://evil.example.com/v.mp4","ext":"mp4","vcodec":"avc1","height":720}`)
	case "slow":
		time.Sleep(20 * time.Second)
	default:
		os.Exit(1)
	}
	os.Exit(0)
}

func fakeYTDLP(t *testing.T, mode string) *atomic.Int32 {
	t.Helper()
	var runs atomic.Int32
	old := ytdlp
	t.Setenv("FAKE_YTDLP", mode)
	ytdlp = func() (string, []string, bool) {
		runs.Add(1)
		return os.Args[0], []string{"-test.run=^TestFakeYTDLP$", "--"}, true
	}
	t.Cleanup(func() { ytdlp = old })
	return &runs
}

func trailerTMDB(t *testing.T) metadata.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[]}`))
		case "/3/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"M11","videos":{"results":[{"key":"teaserKey1","site":"YouTube","type":"Teaser","official":true,"iso_639_1":"en"},{"key":"trailerKey1","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"}]}}`))
		case "/3/movie/12":
			_, _ = w.Write([]byte(`{"id":12,"title":"M12"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// route: GET /api/v1/trailer/{kind}/{tmdb_id}
func TestTrailerStream(t *testing.T) {
	runs := fakeYTDLP(t, "ok")
	srv, _ := discoverRig(t, trailerTMDB(t))
	var s TrailerStream
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.URL, "https://rr1---sn-abc.googlevideo.com/") || s.Key != "trailerKey1" || s.Height != 720 || s.Width != 1280 || !s.Audio ||
		time.Until(s.ExpiresAt) < 5*time.Hour {
		t.Fatalf("stream %+v", s)
	}
	// The address is kept: a second ask does not run yt-dlp again.
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200)
	if runs.Load() != 1 {
		t.Fatalf("yt-dlp ran %d times", runs.Load())
	}
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/12", "", 404) // no trailer
	do(t, "GET", srv.URL+"/api/v1/trailer/artist/1", "", 400)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/x", "", 400)

	// Off for everyone: no trailer at all.
	do(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)
}

func TestTrailerStreamRefusals(t *testing.T) {
	for _, mode := range []string{"fail", "odd"} {
		fakeYTDLP(t, mode)
		srv, _ := discoverRig(t, trailerTMDB(t))
		if b := string(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)); !strings.Contains(b, "no direct stream") {
			t.Fatalf("%s: %s", mode, b)
		}
	}
	// No yt-dlp at all.
	old := ytdlp
	ytdlp = func() (string, []string, bool) { return "", nil, false }
	t.Cleanup(func() { ytdlp = old })
	srv, _ := discoverRig(t, trailerTMDB(t))
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)
}

func TestTrailerStreamTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the 8 second limit")
	}
	fakeYTDLP(t, "slow")
	srv, _ := discoverRig(t, trailerTMDB(t))
	start := time.Now()
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestTrailerBudget(t *testing.T) {
	var ts trailerState
	now := time.Now()
	for i := 0; i < trailerPerMinute; i++ {
		if !ts.allow(7, now) {
			t.Fatalf("refused at %d", i)
		}
	}
	if ts.allow(7, now) || !ts.allow(8, now) || !ts.allow(7, now.Add(61*time.Second)) {
		t.Fatal("budget is per person per minute")
	}
}

// route: GET /api/v1/settings/ui
// route: PUT /api/v1/settings/ui
func TestUISettings(t *testing.T) {
	srv, _ := discoverRig(t, trailerTMDB(t))
	path := srv.URL + "/api/v1/settings/ui"
	if b := string(do(t, "GET", path, "", 200)); !strings.Contains(b, `"autoplay_trailers":true`) {
		t.Fatalf("default %s", b)
	}
	if b := string(do(t, "PUT", path, `{"autoplay_trailers":false}`, 200)); !strings.Contains(b, `"autoplay_trailers":false`) {
		t.Fatalf("off %s", b)
	}
	if b := string(do(t, "PUT", path, `{}`, 200)); !strings.Contains(b, `"autoplay_trailers":false`) {
		t.Fatalf("left out keeps %s", b)
	}
	do(t, "PUT", path, `{"autoplay_trailers":"yes"}`, 400)
	if b := string(do(t, "PUT", path, `{"autoplay_trailers":true}`, 200)); !strings.Contains(b, `"autoplay_trailers":true`) {
		t.Fatalf("on %s", b)
	}
}
