package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/ytdlp"
)

// TestFakeYTDLP is not a test: run as a child process it plays yt-dlp,
// answering with what FAKE_YTDLP says (ok, slow, fail, odd).
func TestFakeYTDLP(t *testing.T) {
	mode := os.Getenv("FAKE_YTDLP")
	if mode == "" {
		return
	}
	args := strings.Join(os.Args, " ")
	want := os.Getenv("FAKE_YTDLP_KEY")
	if want == "" {
		want = "trailerKey1"
	}
	if !strings.Contains(args, "watch?v="+want) || !strings.Contains(args, "player_client=visionos") || !strings.Contains(args, "--no-check-formats") {
		os.Exit(3)
	}
	exp := time.Now().Add(6 * time.Hour).Unix()
	mp4 := fmt.Sprintf(`{"format_id":"137","url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=137","ext":"mp4","vcodec":"avc1.640028","acodec":"none","width":1920,"height":1080,"protocol":"https"},{"format_id":"136","url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=136","ext":"mp4","vcodec":"avc1.4d401f","acodec":"none","width":1280,"height":720,"protocol":"https"},{"format_id":"243","url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=243","ext":"webm","vcodec":"vp9","acodec":"none","height":360,"protocol":"https"}`, exp, exp, exp)
	if mode == "muxed" {
		mode = "mp4"
		mp4 += fmt.Sprintf(`,{"format_id":"18","url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=18","ext":"mp4","vcodec":"avc1.42001E","acodec":"mp4a.40.2","width":640,"height":360,"protocol":"https"}`, exp)
	}
	switch mode {
	case "ok":
		fmt.Printf(`{"formats":[{"format_id":"233","manifest_url":"https://manifest.googlevideo.com/api/manifest/hls_variant/expire/%d/id/x/file/index.m3u8","protocol":"m3u8_native"},%s]}`, exp+60, mp4)
	case "mp4":
		fmt.Printf(`{"formats":[%s]}`, mp4)
	case "odd":
		fmt.Print(`{"formats":[{"format_id":"18","url":"https://evil.example.com/v.mp4","ext":"mp4","vcodec":"avc1"},{"format_id":"1","manifest_url":"https://evil.example.com/hls_variant/x"}]}`)
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
	old := ytdlpRun
	t.Setenv("FAKE_YTDLP", mode)
	oldAllow := ytdlp.EnvAllow
	ytdlp.EnvAllow = append(append([]string{}, oldAllow...), "FAKE_YTDLP", "FAKE_YTDLP_KEY")
	t.Cleanup(func() { ytdlp.EnvAllow = oldAllow })
	ytdlpRun = func() (string, []string, bool) {
		runs.Add(1)
		return os.Args[0], []string{"-test.run=^TestFakeYTDLP$", "--"}, true
	}
	t.Cleanup(func() { ytdlpRun = old })
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
	if s.Type != "hls" || !strings.HasPrefix(s.URL, "https://manifest.googlevideo.com/api/manifest/hls_variant/") || !strings.Contains(s.MP4URL, "itag=136") || s.MP4Audio || s.Key != "trailerKey1" || s.Height != 720 || !s.Audio ||
		time.Until(s.ExpiresAt) < 5*time.Hour || time.Until(s.ExpiresAt) > 6*time.Hour+time.Second {
		t.Fatalf("stream %+v", s)
	}
	// The address is kept: a second ask does not look it up again (ytdlpRun
	// is still asked whether yt-dlp exists, but resolves stay at one).
	before := runs.Load()
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200)
	if runs.Load() != before {
		t.Fatalf("yt-dlp ran again (%d -> %d)", before, runs.Load())
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
	old := ytdlpRun
	ytdlpRun = func() (string, []string, bool) { return "", nil, false }
	t.Cleanup(func() { ytdlpRun = old })
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

// A restart keeps resolved addresses (lookup_cache): a new API on the same
// database answers without yt-dlp. MP4-only answers come back as mp4.
func TestTrailerStreamKeptAcrossRestartAndMP4(t *testing.T) {
	runs := fakeYTDLP(t, "mp4")
	srv, st := discoverRig(t, trailerTMDB(t))
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Type != "mp4" || !strings.Contains(s.URL, "itag=136") || s.MP4URL != "" || s.Audio {
		t.Fatalf("mp4 %+v", s)
	}
	srv2 := httptest.NewServer(New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"}).Handler())
	t.Cleanup(srv2.Close)
	n := runs.Load()
	var again TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv2.URL+"/api/v1/trailer/movie/11", "", 200), &again)
	if again.URL != s.URL || runs.Load() != n {
		t.Fatalf("not kept: %+v runs %d->%d", again, n, runs.Load())
	}
}

// A muxed MP4, when offered, wins over a taller picture-only one.
func TestTrailerStreamPrefersMuxed(t *testing.T) {
	fakeYTDLP(t, "muxed")
	srv, _ := discoverRig(t, trailerTMDB(t))
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Type != "mp4" || !strings.Contains(s.URL, "itag=18") || !s.Audio {
		t.Fatalf("%+v", s)
	}
}

// The metadata server's pick wins over TMDB's videos; its "no trailer"
// is final.
func TestTrailerUsesServerPick(t *testing.T) {
	fakeYTDLP(t, "ok")
	t.Setenv("FAKE_YTDLP_KEY", "serverPick1")
	_, st := discoverRig(t, trailerTMDB(t))
	srv := httptest.NewServer(New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test", TrailerPick: func(_ context.Context, kind, id, lang string) (string, error) {
		if id == "12" {
			return "", metadata.ErrNotFound
		}
		return "serverPick1", nil
	}}).Handler())
	t.Cleanup(srv.Close)
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Key != "serverPick1" {
		t.Fatalf("%+v", s)
	}
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/12", "", 404)
}

// Many asks for one title share one lookup; failures in a row are counted
// for the health check and a success resets them.
func TestTrailerSharedLookupAndFailures(t *testing.T) {
	fakeYTDLP(t, "fail")
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	for range 3 {
		_, _ = a.resolveTitle(ctx, "movie", "11", "en")
		a.trailers.cache = nil // forget the miss, as after trailerMiss
	}
	if n, on := a.TrailerFailures(); n != 3 || !on {
		t.Fatalf("failures %d %v", n, on)
	}
	runs := fakeYTDLP(t, "ok")
	a.trailers.cache = nil
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s, err := a.resolveTitle(ctx, "movie", "11", "en"); err != nil || s.Type != "hls" {
				t.Errorf("%+v %v", s, err)
			}
		}()
	}
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("lookups %d", runs.Load())
	}
	if n, _ := a.TrailerFailures(); n != 0 {
		t.Fatalf("not reset %d", n)
	}
}

func TestExploreTrailerTitles(t *testing.T) {
	it := func(kind, id string) discoverItemView {
		return discoverItemView{DiscoverItem: metadata.DiscoverItem{Kind: kind, TMDBID: id}}
	}
	hero := []discoverItemView{it("movie", "1"), it("show", "2")}
	head := []exploreRowView{{Items: []discoverItemView{}}, {Items: []discoverItemView{it("movie", "1"), it("movie", "3"), it("movie", "4"), it("movie", "5"), it("movie", "6")}}, {Items: []discoverItemView{it("artist", "x"), it("show", "7")}}, {Items: []discoverItemView{it("movie", "8")}}, {Items: []discoverItemView{it("movie", "9")}}}
	got := fmt.Sprint(exploreTrailerTitles(hero, head))
	if got != "[[movie 1] [show 2] [movie 3] [movie 4] [movie 5] [show 7] [movie 8]]" {
		t.Fatal(got)
	}
}
