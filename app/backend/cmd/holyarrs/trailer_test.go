package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// ytFake plays YouTube for the trailer reader: the watch page and the
// player call answer each video by its mode, and any other video as
// removed. Modes: ok (HLS and picture-only MP4s), heat and plain (200 s
// long, with or without a "most replayed" graph whose opening is the
// highest part), mp4 (no HLS), muxed (an MP4 with sound too), odd
// (addresses off YouTube's video servers), junk (an unreadable answer),
// fail (an error status), slow (no answer until the lookup gives up),
// age, unavailable and bot.
type ytFake struct {
	runs  atomic.Int32 // player calls
	pages atomic.Int32 // watch pages read
	mu    sync.Mutex
	modes map[string]string
	asked map[string]int
	stop  chan struct{}
}

// fakeReader points the trailer reader at a fake YouTube where keys (else
// trailerKey1) answer in mode.
func fakeReader(t *testing.T, mode string, keys ...string) *ytFake {
	t.Helper()
	if len(keys) == 0 {
		keys = []string{"trailerKey1"}
	}
	f := &ytFake{modes: map[string]string{}, asked: map[string]int{}, stop: make(chan struct{})}
	for _, k := range keys {
		f.modes[k] = mode
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(f.stop) }) // runs first: a slow answer ends
	old := metadata.UseReader(&youtube.Reader{Base: srv.URL, HTTP: srv.Client()})
	t.Cleanup(func() { metadata.UseReader(old) })
	return f
}

func (f *ytFake) set(key, mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modes[key] = mode
}

// count is how many player calls asked for key.
func (f *ytFake) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked[key]
}

func (f *ytFake) serve(w http.ResponseWriter, r *http.Request) {
	var id string
	switch r.URL.Path {
	case "/watch":
		id = r.URL.Query().Get("v")
		f.pages.Add(1)
	case "/youtubei/v1/player":
		var b struct {
			VideoID string `json:"videoId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&b)
		id = b.VideoID
	default:
		w.WriteHeader(404)
		return
	}
	f.mu.Lock()
	mode, ok := f.modes[id]
	if !ok {
		mode = "unavailable"
	}
	if r.URL.Path != "/watch" {
		f.asked[id]++
		f.runs.Add(1)
	}
	f.mu.Unlock()
	if r.URL.Path == "/watch" {
		_, _ = io.WriteString(w, fakeWatchPage(mode))
		return
	}
	if mode == "slow" {
		select {
		case <-r.Context().Done():
		case <-f.stop:
		case <-time.After(20 * time.Second):
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	code, body := fakePlayer(mode)
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body)
}

// fakeWatchPage has a visitor id and, for heat, a "most replayed" graph of
// 20 s parts whose opening is the highest and whose real peak is at 80 s.
func fakeWatchPage(mode string) string {
	page := `<html><script>ytcfg.set({"VISITOR_DATA":"VISITOR-FAKE"});</script>`
	if mode == "heat" {
		var marks []string
		for i := range 10 {
			v := 0.2
			switch i {
			case 0:
				v = 1
			case 4:
				v = 0.9
			}
			marks = append(marks, fmt.Sprintf(`{"startMillis":"%d","durationMillis":"20000","intensityScoreNormalized":%v}`, i*20000, v))
		}
		page += `<script>var ytInitialData = {"frameworkUpdates":{"entityBatchUpdate":{"mutations":[{"payload":{"macroMarkersListEntity":{"markersList":{"markerType":"MARKER_TYPE_HEATMAP","markers":[` + strings.Join(marks, ",") + `]}}}}]}}};</script>`
	}
	return page + "</html>"
}

func fakePlayer(mode string) (int, string) {
	exp := time.Now().Add(6 * time.Hour).Unix()
	refuse := func(status, reason string) (int, string) {
		return 200, `{"playabilityStatus":{"status":"` + status + `","reason":"` + reason + `"}}`
	}
	hls := fmt.Sprintf(`"hlsManifestUrl":"https://manifest.googlevideo.com/api/manifest/hls_variant/expire/%d/id/x/file/index.m3u8",`, exp+60)
	adaptive := fmt.Sprintf(`"adaptiveFormats":[{"itag":137,"url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=137","mimeType":"video/mp4; codecs=\"avc1.640028\"","width":1920,"height":1080},`+
		`{"itag":136,"url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=136","mimeType":"video/mp4; codecs=\"avc1.4d401f\"","width":1280,"height":720},`+
		`{"itag":243,"url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=243","mimeType":"video/webm; codecs=\"vp9\"","width":640,"height":360}]`, exp, exp, exp)
	details := ""
	streaming := hls + adaptive
	switch mode {
	case "ok":
	case "heat", "plain":
		details = `"videoDetails":{"title":"Fake","lengthSeconds":"200"},`
	case "mp4":
		streaming = adaptive
	case "muxed":
		streaming = adaptive + fmt.Sprintf(`,"formats":[{"itag":18,"url":"https://rr1---sn-abc.googlevideo.com/videoplayback?expire=%d&itag=18","mimeType":"video/mp4; codecs=\"avc1.42001E, mp4a.40.2\"","width":640,"height":360}]`, exp)
	case "odd":
		streaming = `"hlsManifestUrl":"https://evil.example.com/hls_variant/x","formats":[{"itag":18,"url":"https://evil.example.com/v.mp4","mimeType":"video/mp4; codecs=\"avc1.42001E, mp4a.40.2\"","height":360}]`
	case "junk":
		return 200, "<html>not an answer"
	case "age":
		return refuse("LOGIN_REQUIRED", "Sign in to confirm your age")
	case "bot":
		return refuse("LOGIN_REQUIRED", "Sign in to confirm you're not a bot")
	case "unavailable":
		return refuse("ERROR", "Video unavailable")
	case "premiere":
		return refuse("LIVE_STREAM_OFFLINE", "Premieres in 2 hours")
	case "later":
		return refuse("ERROR", "This content isn't available, try again later.")
	default:
		return 500, "{}"
	}
	return 200, `{"playabilityStatus":{"status":"OK"},` + details + `"streamingData":{` + streaming + `}}`
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
		case "/3/movie/13", "/3/movie/14", "/3/movie/15":
			// Several trailers: 13 and 14 have three, 15 four.
			id := strings.TrimPrefix(r.URL.Path, "/3/movie/")
			more := ""
			if id == "15" {
				more = `,{"key":"fourthKey01","site":"YouTube","type":"Teaser","official":false,"iso_639_1":"en"}`
			}
			_, _ = w.Write([]byte(`{"id":` + id + `,"title":"M` + id + `","videos":{"results":[{"key":"ageGated001","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"},{"key":"secondKey01","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"},{"key":"thirdKey001","site":"YouTube","type":"Teaser","official":true,"iso_639_1":"en"}` + more + `]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}

// trailerRig is an API on the discover rig's database, with its server.
func trailerRig(t *testing.T) (*httptest.Server, *handlers) {
	t.Helper()
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return srv, a
}

// lookUp looks a title up as the trailer pass does; the routes then read
// what it kept.
func lookUp(t *testing.T, a *handlers, kind, id string) error {
	t.Helper()
	_, err := a.Trailers().ResolveTitle(context.Background(), kind, id, "en")
	return err
}

// route: GET /api/v1/trailer/{kind}/{tmdb_id}
func TestTrailerStream(t *testing.T) {
	yt := fakeReader(t, "ok")
	srv, a := trailerRig(t)
	// Not looked up yet: "no direct stream" at once, without a lookup.
	var cold struct{ Error, Reason, Key string }
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404), &cold)
	if cold.Error != "no direct stream" || cold.Reason != "" || yt.runs.Load() != 0 {
		t.Fatalf("cold %+v, lookups %d", cold, yt.runs.Load())
	}
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	var s metadata.TrailerStream
	if err := json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s); err != nil {
		t.Fatal(err)
	}
	if s.Type != "hls" || !strings.HasPrefix(s.URL, "https://manifest.googlevideo.com/api/manifest/hls_variant/") || !strings.Contains(s.MP4URL, "itag=136") || s.MP4Audio || s.Key != "trailerKey1" || s.Height != 720 || !s.Audio ||
		time.Until(s.ExpiresAt) < 5*time.Hour || time.Until(s.ExpiresAt) > 6*time.Hour+time.Second {
		t.Fatalf("stream %+v", s)
	}
	// The address is kept: a second ask does not look it up again.
	before := yt.runs.Load()
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200)
	if yt.runs.Load() != before {
		t.Fatalf("looked up again (%d -> %d)", before, yt.runs.Load())
	}
	// Another language was not looked up.
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11?language=de", "", 404)
	if err := lookUp(t, a, "movie", "12"); err != metadata.ErrNoTrailer {
		t.Fatalf("no trailer: %v", err)
	}
	if b := string(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/12", "", 404)); !strings.Contains(b, `"no trailer"`) {
		t.Fatal(b)
	}
	if yt.runs.Load() != before {
		t.Fatalf("a route looked up (%d -> %d)", before, yt.runs.Load())
	}
	trDo(t, "GET", srv.URL+"/api/v1/trailer/artist/1", "", 400)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/x", "", 400)

	// Off for everyone: no trailer at all.
	trDo(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)
}

// A lookup that finds no stream answers 404 with the reader's reason. At
// YouTube's door (a bot check, an error, an odd or unreadable answer) the
// key is still given: YouTube's own player may play it.
func TestTrailerStreamRefusals(t *testing.T) {
	for mode, reason := range map[string]youtube.Reason{"fail": youtube.Refused, "odd": youtube.NoAddress, "junk": youtube.Network, "bot": youtube.BotCheck} {
		fakeReader(t, mode)
		srv, a := trailerRig(t)
		_ = lookUp(t, a, "movie", "11")
		var b struct{ Error, Reason, Key string }
		_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404), &b)
		if b.Error != "no direct stream" || b.Reason != string(reason) || b.Key != "trailerKey1" {
			t.Fatalf("%s: %+v", mode, b)
		}
	}
}

func TestTrailerStreamTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for the 8 second limit")
	}
	fakeReader(t, "slow")
	_, a := trailerRig(t)
	start := time.Now()
	m := (*metadata.TrailerMissed)(nil)
	if err := lookUp(t, a, "movie", "11"); !errors.As(err, &m) {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("took %v", d)
	}
}

// route: GET /api/v1/settings/ui
// route: PUT /api/v1/settings/ui
func TestUISettings(t *testing.T) {
	srv, _ := trDiscoverRig(t, trailerTMDB(t))
	path := srv.URL + "/api/v1/settings/ui"
	if b := string(trDo(t, "GET", path, "", 200)); !strings.Contains(b, `"autoplay_trailers":true`) {
		t.Fatalf("default %s", b)
	}
	if b := string(trDo(t, "PUT", path, `{"autoplay_trailers":false}`, 200)); !strings.Contains(b, `"autoplay_trailers":false`) {
		t.Fatalf("off %s", b)
	}
	if b := string(trDo(t, "PUT", path, `{}`, 200)); !strings.Contains(b, `"autoplay_trailers":false`) {
		t.Fatalf("left out keeps %s", b)
	}
	trDo(t, "PUT", path, `{"autoplay_trailers":"yes"}`, 400)
	if b := string(trDo(t, "PUT", path, `{"autoplay_trailers":true}`, 200)); !strings.Contains(b, `"autoplay_trailers":true`) {
		t.Fatalf("on %s", b)
	}
}

// A restart keeps resolved addresses (lookup_cache): a new API on the same
// database answers without a lookup. MP4-only answers come back as mp4.
func TestTrailerStreamKeptAcrossRestartAndMP4(t *testing.T) {
	yt := fakeReader(t, "mp4")
	srv, a := trailerRig(t)
	st := a.Trailers().Store
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	var s metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Type != "mp4" || !strings.Contains(s.URL, "itag=136") || s.MP4URL != "" || s.Audio {
		t.Fatalf("mp4 %+v", s)
	}
	srv2 := httptest.NewServer(buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"}).Handler())
	t.Cleanup(srv2.Close)
	n := yt.runs.Load()
	var again metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv2.URL+"/api/v1/trailer/movie/11", "", 200), &again)
	if again.URL != s.URL || yt.runs.Load() != n {
		t.Fatalf("not kept: %+v runs %d->%d", again, n, yt.runs.Load())
	}
}

// A muxed MP4, when offered, wins over a taller picture-only one.
func TestTrailerStreamPrefersMuxed(t *testing.T) {
	fakeReader(t, "muxed")
	srv, a := trailerRig(t)
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	var s metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Type != "mp4" || !strings.Contains(s.URL, "itag=18") || !s.Audio {
		t.Fatalf("%+v", s)
	}
}

// The metadata server's pick wins over TMDB's videos; its "no trailer"
// is final.
func TestTrailerUsesServerPick(t *testing.T) {
	fakeReader(t, "ok", "serverPick1")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test", TrailerPick: func(_ context.Context, kind, id, lang string) (string, error) {
		if id == "12" {
			return "", metadata.ErrNotFound
		}
		return "serverPick1", nil
	}})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	if err := lookUp(t, a, "movie", "11"); err != nil {
		t.Fatal(err)
	}
	if err := lookUp(t, a, "movie", "12"); err != metadata.ErrNoTrailer {
		t.Fatal(err)
	}
	var s metadata.TrailerStream
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Key != "serverPick1" {
		t.Fatalf("%+v", s)
	}
	trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/12", "", 404)
}

// Many asks for one title share one lookup; failures in a row are counted
// for the health check and a success resets them.
func TestTrailerSharedLookupAndFailures(t *testing.T) {
	fakeReader(t, "fail")
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	for range 3 {
		_, _ = a.Trailers().ResolveTitle(ctx, "movie", "11", "en")
		a.Trailers().Forget() // forget the miss, as after metadata.TrailerMiss
	}
	if st := a.Trailers().ReaderState(); st.Failed != 3 || st.LastReason != youtube.Refused || !st.LastGood.IsZero() {
		t.Fatalf("failures %+v", st)
	}
	yt := fakeReader(t, "ok")
	a.Trailers().Forget()
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s, err := a.Trailers().ResolveTitle(ctx, "movie", "11", "en"); err != nil || s.Type != "hls" {
				t.Errorf("%+v %v", s, err)
			}
		}()
	}
	wg.Wait()
	if yt.runs.Load() != 1 {
		t.Fatalf("lookups %d", yt.runs.Load())
	}
	if st := a.Trailers().ReaderState(); st.Failed != 0 || time.Since(st.LastGood) > time.Minute {
		t.Fatalf("not reset %+v", st)
	}
}

func TestExploreTrailerTitles(t *testing.T) {
	it := func(kind, id string) engine.Item { return engine.Item{Kind: kind, TMDBID: id} }
	row := func(items ...engine.Item) engine.Row { return engine.Row{Items: items} }
	hero := []engine.Item{it("movie", "1"), it("show", "2")}
	head := []engine.Row{row(), row(it("movie", "1"), it("movie", "3"), it("movie", "4"), it("movie", "5"), it("movie", "6")), row(it("artist", "x"), it("show", "7")), row(it("movie", "8")), row(it("movie", "9"))}
	got := fmt.Sprint(discover.ExploreTrailerTitles(hero, head))
	if got != "[[movie 1] [show 2] [movie 3] [movie 4] [movie 5] [show 7] [movie 8]]" {
		t.Fatal(got)
	}
}

// ?language= is "xx" or "xx-YY"; anything else falls back to Accept-Language.
func TestTrailerLanguageChecked(t *testing.T) {
	for q, want := range map[string]string{"de": "de", "pt-BR": "pt", "|x": "fr", "DE": "fr", "de-br": "fr", "dee": "fr", "": "fr"} {
		r := httptest.NewRequest("GET", "/x?language="+url.QueryEscape(q), nil)
		r.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")
		if got := metadata.TrailerLanguage(r); got != want {
			t.Fatalf("%q: %q", q, got)
		}
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Accept-Language", "|x")
	if got := metadata.TrailerLanguage(r); got != "en" {
		t.Fatalf("junk header: %q", got)
	}
}

// A kept address is checked again when read back from SQLite.
func TestTrailerStoredRechecked(t *testing.T) {
	_, st := trDiscoverRig(t, trailerTMDB(t))
	a := buildHandlers(services{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx, now := context.Background(), time.Now()
	later := now.Add(time.Hour)
	put := func(k string, s metadata.TrailerStream) {
		b, _ := json.Marshal(s)
		if err := (metadata.LookupCache{Store: st}).Put(ctx, "trailer|"+k, b, now, later); err != nil {
			t.Fatal(err)
		}
	}
	good := "https://rr1.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(later.Unix(), 10)
	put("movie|1|en", metadata.TrailerStream{Type: "mp4", URL: good, ExpiresAt: later})
	put("movie|2|en", metadata.TrailerStream{Type: "mp4", URL: "https://evil.example/v", ExpiresAt: later})
	put("movie|3|en", metadata.TrailerStream{Type: "hls", URL: good, MP4URL: "http://rr1.googlevideo.com/v", ExpiresAt: later})
	put("movie|4|en", metadata.TrailerStream{Type: "mp4", URL: "https://rr1.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(now.Add(-time.Minute).Unix(), 10), ExpiresAt: later})
	for k, want := range map[string]bool{"movie|1|en": true, "movie|2|en": false, "movie|3|en": false, "movie|4|en": false} {
		if got := a.Trailers().Stored(ctx, k, now); got != want {
			t.Fatalf("%s: %v", k, got)
		}
	}
}

// A page open, a batch and Explore's routes never look a trailer up on
// YouTube: a title not looked up yet answers "no direct stream" with its
// kept key, the batch leaves it out with nothing pending, and Explore's
// title warmer learns the key only.
func TestTrailerPageOpensNeverLookUp(t *testing.T) {
	yt := fakeReader(t, "ok")
	srv, a := trailerRig(t)
	a.Trailers().KeepKey(context.Background(), "movie", "11", "en", "trailerKey1")
	var b struct{ Error, Reason, Key string }
	_ = json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404), &b)
	if b.Error != "no direct stream" || b.Reason != "" || b.Key != "trailerKey1" {
		t.Fatalf("cold %+v", b)
	}
	var batch trailerBatchView
	if err := json.Unmarshal(trDo(t, "GET", srv.URL+"/api/v1/trailer/batch?ids=movie:11,show:22", "", 200), &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Streams) != 0 || len(batch.Pending) != 0 {
		t.Fatalf("batch %+v", batch)
	}

	ea, _, _ := trExploreRig(t, func(context.Context, string, string, string) (string, error) { return "trailerKey1", nil })
	ctx := context.Background()
	for _, sw := range discover.ExploreSwitches {
		if err := ea.TrailerTest().Explore().BuildPool(ctx, 1, sw); err != nil {
			t.Fatal(err)
		}
	}
	esrv := httptest.NewServer(ea.Handler())
	t.Cleanup(esrv.Close)
	head := trGetJSON(t, esrv.URL+"/api/v1/explore/head?type=all", 200)
	trGetJSON(t, esrv.URL+"/api/v1/explore/tail?type=all&page=0", 200)
	trGetJSON(t, esrv.URL+"/api/v1/explore/new?type=all", 200)
	// The warmer runs beside the page: once it learned a listed title's key
	// (its last step), nothing it does looks the trailer up.
	var first [2]string
	for _, row := range head["head"].([]any) {
		for _, v := range row.(map[string]any)["items"].([]any) {
			if m := v.(map[string]any); first[0] == "" && (m["kind"] == "movie" || m["kind"] == "show") {
				first = [2]string{m["kind"].(string), m["tmdb_id"].(string)}
			}
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for first[0] != "" && ea.Trailers().KnownKey(ctx, first[0], first[1], "en") == "" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if first[0] == "" || ea.Trailers().KnownKey(ctx, first[0], first[1], "en") != "trailerKey1" {
		t.Fatalf("the warmer did not learn %v's key", first)
	}
	time.Sleep(200 * time.Millisecond)
	if n, p := yt.runs.Load(), yt.pages.Load(); n != 0 || p != 0 {
		t.Fatalf("page opens looked up: %d player calls, %d watch pages", n, p)
	}
}
