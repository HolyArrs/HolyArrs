package api

import (
	"context"
	"encoding/json"
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

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/youtube"
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
	old := trailerReader
	trailerReader = &youtube.Reader{Base: srv.URL, HTTP: srv.Client()}
	t.Cleanup(func() { trailerReader = old })
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

// route: GET /api/v1/trailer/{kind}/{tmdb_id}
func TestTrailerStream(t *testing.T) {
	yt := fakeReader(t, "ok")
	srv, _ := discoverRig(t, trailerTMDB(t))
	var s TrailerStream
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s); err != nil {
		t.Fatal(err)
	}
	if s.Type != "hls" || !strings.HasPrefix(s.URL, "https://manifest.googlevideo.com/api/manifest/hls_variant/") || !strings.Contains(s.MP4URL, "itag=136") || s.MP4Audio || s.Key != "trailerKey1" || s.Height != 720 || !s.Audio ||
		time.Until(s.ExpiresAt) < 5*time.Hour || time.Until(s.ExpiresAt) > 6*time.Hour+time.Second {
		t.Fatalf("stream %+v", s)
	}
	// The address is kept: a second ask does not look it up again.
	before := yt.runs.Load()
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200)
	if yt.runs.Load() != before {
		t.Fatalf("looked up again (%d -> %d)", before, yt.runs.Load())
	}
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/12", "", 404) // no trailer
	do(t, "GET", srv.URL+"/api/v1/trailer/artist/1", "", 400)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/x", "", 400)

	// Off for everyone: no trailer at all.
	do(t, "PUT", srv.URL+"/api/v1/settings/ui", `{"autoplay_trailers":false}`, 200)
	do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)
}

// A lookup that finds no stream answers 404 with the reader's reason. At
// YouTube's door (a bot check, an error, an odd or unreadable answer) the
// key is still given: YouTube's own player may play it.
func TestTrailerStreamRefusals(t *testing.T) {
	for mode, reason := range map[string]youtube.Reason{"fail": youtube.Refused, "odd": youtube.NoAddress, "junk": youtube.Network, "bot": youtube.BotCheck} {
		fakeReader(t, mode)
		srv, _ := discoverRig(t, trailerTMDB(t))
		var b struct{ Error, Reason, Key string }
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404), &b)
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
// database answers without a lookup. MP4-only answers come back as mp4.
func TestTrailerStreamKeptAcrossRestartAndMP4(t *testing.T) {
	yt := fakeReader(t, "mp4")
	srv, st := discoverRig(t, trailerTMDB(t))
	var s TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 200), &s)
	if s.Type != "mp4" || !strings.Contains(s.URL, "itag=136") || s.MP4URL != "" || s.Audio {
		t.Fatalf("mp4 %+v", s)
	}
	srv2 := httptest.NewServer(New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"}).Handler())
	t.Cleanup(srv2.Close)
	n := yt.runs.Load()
	var again TrailerStream
	_ = json.Unmarshal(do(t, "GET", srv2.URL+"/api/v1/trailer/movie/11", "", 200), &again)
	if again.URL != s.URL || yt.runs.Load() != n {
		t.Fatalf("not kept: %+v runs %d->%d", again, n, yt.runs.Load())
	}
}

// A muxed MP4, when offered, wins over a taller picture-only one.
func TestTrailerStreamPrefersMuxed(t *testing.T) {
	fakeReader(t, "muxed")
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
	fakeReader(t, "ok", "serverPick1")
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
	fakeReader(t, "fail")
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx := context.Background()
	for range 3 {
		_, _ = a.resolveTitle(ctx, "movie", "11", "en")
		a.trailers.cache = nil // forget the miss, as after trailerMiss
	}
	if st := a.TrailerReader(); st.Failed != 3 || st.LastReason != youtube.Refused || !st.LastGood.IsZero() {
		t.Fatalf("failures %+v", st)
	}
	yt := fakeReader(t, "ok")
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
	if yt.runs.Load() != 1 {
		t.Fatalf("lookups %d", yt.runs.Load())
	}
	if st := a.TrailerReader(); st.Failed != 0 || time.Since(st.LastGood) > time.Minute {
		t.Fatalf("not reset %+v", st)
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

// ?language= is "xx" or "xx-YY"; anything else falls back to Accept-Language.
func TestTrailerLanguageChecked(t *testing.T) {
	for q, want := range map[string]string{"de": "de", "pt-BR": "pt", "|x": "fr", "DE": "fr", "de-br": "fr", "dee": "fr", "": "fr"} {
		r := httptest.NewRequest("GET", "/x?language="+url.QueryEscape(q), nil)
		r.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")
		if got := trailerLanguage(r); got != want {
			t.Fatalf("%q: %q", q, got)
		}
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Accept-Language", "|x")
	if got := trailerLanguage(r); got != "en" {
		t.Fatalf("junk header: %q", got)
	}
}

// A kept address is checked again when read back from SQLite.
func TestTrailerStoredRechecked(t *testing.T) {
	_, st := discoverRig(t, trailerTMDB(t))
	a := New(Server{Store: st, Metadata: trailerTMDB(t), Version: "test"})
	ctx, now := context.Background(), time.Now()
	later := now.Add(time.Hour)
	put := func(k string, s TrailerStream) {
		b, _ := json.Marshal(s)
		if err := (store.LookupCache{Store: st}).Put(ctx, "trailer|"+k, b, now, later); err != nil {
			t.Fatal(err)
		}
	}
	good := "https://rr1.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(later.Unix(), 10)
	put("movie|1|en", TrailerStream{Type: "mp4", URL: good, ExpiresAt: later})
	put("movie|2|en", TrailerStream{Type: "mp4", URL: "https://evil.example/v", ExpiresAt: later})
	put("movie|3|en", TrailerStream{Type: "hls", URL: good, MP4URL: "http://rr1.googlevideo.com/v", ExpiresAt: later})
	put("movie|4|en", TrailerStream{Type: "mp4", URL: "https://rr1.googlevideo.com/videoplayback?expire=" + strconv.FormatInt(now.Add(-time.Minute).Unix(), 10), ExpiresAt: later})
	for k, want := range map[string]bool{"movie|1|en": true, "movie|2|en": false, "movie|3|en": false, "movie|4|en": false} {
		if got := a.trailerStored(ctx, k, now); got != want {
			t.Fatalf("%s: %v", k, got)
		}
	}
}

// The tests above wait for the lookup; the page itself never does.
func init() { trailerWait = 30 * time.Second }

func TestTrailerNeverWaitsOnLookup(t *testing.T) {
	old := trailerWait
	trailerWait = 50 * time.Millisecond
	t.Cleanup(func() { trailerWait = old })
	fakeReader(t, "slow")
	srv, _ := discoverRig(t, trailerTMDB(t))
	start := time.Now()
	if b := string(do(t, "GET", srv.URL+"/api/v1/trailer/movie/11", "", 404)); !strings.Contains(b, "no trailer yet") {
		t.Fatal(b)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("waited %v", d)
	}
}
