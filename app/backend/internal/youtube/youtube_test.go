package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const vid = "aqz-KE-bpKQ"

// fakeYouTube serves a watch page and the player call. play answers the
// player call for a visitor id; page answers the page (nil: a normal one).
type fakeYouTube struct {
	mu      sync.Mutex
	players []map[string]any // player call bodies
	headers []http.Header
	pages   []string // page queries
	play    func(visitor string) (int, string)
	page    func() (int, string)
}

func (f *fakeYouTube) serve(t *testing.T) *Reader {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/watch":
			f.mu.Lock()
			f.pages = append(f.pages, r.URL.RawQuery)
			f.mu.Unlock()
			code, body := 200, watchHTML("VISITOR-NEW", 3)
			if f.page != nil {
				code, body = f.page()
			}
			w.WriteHeader(code)
			io.WriteString(w, body)
		case "/youtubei/v1/player":
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			f.mu.Lock()
			f.players = append(f.players, b)
			f.headers = append(f.headers, r.Header.Clone())
			f.mu.Unlock()
			code, body := f.play(r.Header.Get("X-Goog-Visitor-Id"))
			w.WriteHeader(code)
			io.WriteString(w, body)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &Reader{Base: srv.URL, HTTP: srv.Client()}
}

// watchHTML is a watch page with a visitor id, a storyboard spec and a
// "most replayed" graph whose opening is the highest part and whose real
// peak starts at peakAt*10 seconds.
func watchHTML(visitor string, peakAt int) string {
	var marks []string
	for i := 0; i < 10; i++ {
		v := 0.2
		if i == 0 {
			v = 1
		} else if i == peakAt {
			v = 0.9
		}
		marks = append(marks, fmt.Sprintf(`{"startMillis":"%d","durationMillis":"10000","intensityScoreNormalized":%v}`, i*10000, v))
	}
	return `<html><script>ytcfg.set({"VISITOR_DATA":"` + visitor + `"});</script>` +
		`<script>var ytInitialPlayerResponse = {"videoDetails":{"title":"Page title","lengthSeconds":"100"},"storyboards":{"playerStoryboardSpecRenderer":{"spec":"https://i.ytimg.com/sb/x/$L/$N.jpg|48#27#100#10#10#0#default#rs$A|80#45#100#10#10#2000#M$M#rs$B"}}};</script>` +
		`<script>var ytInitialData = {"frameworkUpdates":{"entityBatchUpdate":{"mutations":[{"payload":{"macroMarkersListEntity":{"markersList":{"markerType":"MARKER_TYPE_HEATMAP","markers":[` + strings.Join(marks, ",") + `]}}}}]}}};</script></html>`
}

func okPlayer(string) (int, string) {
	return 200, `{"playabilityStatus":{"status":"OK"},"videoDetails":{"title":"Big Buck Bunny","lengthSeconds":"634"},
	"storyboards":{"playerStoryboardSpecRenderer":{"spec":"short-spec"}},
	"streamingData":{"hlsManifestUrl":"https://manifest.googlevideo.com/api/manifest/hls_variant/expire/1999999999/x/index.m3u8",
	"formats":[{"itag":18,"signatureCipher":"s=abc&url=x","mimeType":"video/mp4; codecs=\"avc1.42001E, mp4a.40.2\""}],
	"adaptiveFormats":[{"itag":136,"url":"https://rr1.googlevideo.com/videoplayback?expire=1999999999&itag=136","mimeType":"video/mp4; codecs=\"avc1.4d401f\"","width":1280,"height":720},
	{"itag":247,"url":"https://rr1.googlevideo.com/videoplayback?itag=247","mimeType":"video/webm; codecs=\"vp9\"","width":1280,"height":720}]}}`
}

func status(st, reason string) func(string) (int, string) {
	return func(string) (int, string) {
		return 200, `{"playabilityStatus":{"status":"` + st + `","reason":"` + reason + `"}}`
	}
}

func TestLookFindsAddressesPeakAndStoryboard(t *testing.T) {
	f := &fakeYouTube{play: okPlayer}
	r := f.serve(t)
	v, err := r.Look(context.Background(), vid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.HLS, "/hls_variant/") || len(v.Formats) != 1 || v.Formats[0].Height != 720 || !v.Formats[0].AVC || v.Formats[0].Audio {
		t.Fatalf("addresses %+v", v)
	}
	if v.Duration != 634 || v.Title != "Big Buck Bunny" {
		t.Fatalf("details %v %q", v.Duration, v.Title)
	}
	// The opening is the highest part; the real peak (30 s) wins.
	if !v.PeakFound || v.Peak != 30 {
		t.Fatalf("peak %v %v", v.Peak, v.PeakFound)
	}
	if !strings.HasPrefix(v.Storyboard, "https://i.ytimg.com/sb/") {
		t.Fatalf("storyboard from the page expected: %q", v.Storyboard)
	}
	// No visitor id yet: the page first, then the player call with its id.
	if len(f.players) != 1 || f.headers[0].Get("X-Goog-Visitor-Id") != "VISITOR-NEW" || f.headers[0].Get("X-YouTube-Client-Name") != Client.Number {
		t.Fatalf("player call %v", f.headers)
	}
}

// No age-gate or content-warning bypass is ever asked for, and no cookie.
func TestLookAsksForNoBypass(t *testing.T) {
	f := &fakeYouTube{play: okPlayer}
	r := f.serve(t)
	if _, err := r.Look(context.Background(), vid); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"contentCheckOk", "racyCheckOk"} {
		if _, ok := f.players[0][k]; ok {
			t.Fatalf("player call asks %s", k)
		}
	}
	for _, q := range f.pages {
		if strings.Contains(q, "bpctr") || strings.Contains(q, "has_verified") {
			t.Fatalf("watch page asks a bypass: %s", q)
		}
	}
	if f.headers[0].Get("Cookie") != "" || f.headers[0].Get("Authorization") != "" {
		t.Fatal("a cookie or login was sent")
	}
}

func TestLookReasons(t *testing.T) {
	for _, c := range []struct {
		name  string
		play  func(string) (int, string)
		want  Reason
		video bool
	}{
		{"age", status("LOGIN_REQUIRED", "Sign in to confirm your age"), AgeRestricted, true},
		{"age check", status("AGE_CHECK_REQUIRED", ""), AgeRestricted, true},
		{"missing", status("ERROR", "This video is unavailable"), Unavailable, true},
		{"private", status("LOGIN_REQUIRED", "This video is private"), SignInNeeded, true},
		{"blocked", status("UNPLAYABLE", "The uploader has not made this video available in your country"), Unplayable, true},
		{"warning", status("CONTENT_CHECK_REQUIRED", "This video may be inappropriate for some users."), ContentCheck, true},
		{"bot", status("LOGIN_REQUIRED", "Sign in to confirm you're not a bot"), BotCheck, false},
		{"scrambled", func(string) (int, string) {
			return 200, `{"playabilityStatus":{"status":"OK"},"streamingData":{"formats":[{"signatureCipher":"s=1&url=x","mimeType":"video/mp4"}]}}`
		}, NoAddress, false},
		{"sabr", func(string) (int, string) {
			return 200, `{"playabilityStatus":{"status":"OK"},"streamingData":{"serverAbrStreamingUrl":"https://x.googlevideo.com/sabr"}}`
		}, NoAddress, false},
		{"busy", func(string) (int, string) { return 429, "" }, Refused, false},
		{"garbage", func(string) (int, string) { return 200, "<html>" }, Network, false},
	} {
		f := &fakeYouTube{play: c.play}
		_, err := f.serve(t).Look(context.Background(), vid)
		var e *Error
		if !errors.As(err, &e) || e.Reason != c.want || e.Reason.VideoReason() != c.video {
			t.Fatalf("%s: %v", c.name, err)
		}
		if ReasonOf(err) != c.want {
			t.Fatalf("%s: ReasonOf %q", c.name, ReasonOf(err))
		}
	}
	// YouTube's own words are kept for Health.
	f := &fakeYouTube{play: status("LOGIN_REQUIRED", "Sign in to confirm your age")}
	if _, err := f.serve(t).Look(context.Background(), vid); !strings.Contains(err.Error(), "confirm your age") {
		t.Fatal(err)
	}
}

// A kept visitor id YouTube no longer takes is renewed from the page read
// at the same time, and the player call is made once more.
func TestLookRenewsAStaleVisitor(t *testing.T) {
	f := &fakeYouTube{play: func(v string) (int, string) {
		if v != "VISITOR-NEW" {
			return status("LOGIN_REQUIRED", "Sign in to confirm you're not a bot")(v)
		}
		return okPlayer(v)
	}}
	r := f.serve(t)
	r.keepVisitor("VISITOR-OLD")
	if _, err := r.Look(context.Background(), vid); err != nil {
		t.Fatal(err)
	}
	if len(f.players) != 2 || f.headers[0].Get("X-Goog-Visitor-Id") != "VISITOR-OLD" || f.headers[1].Get("X-Goog-Visitor-Id") != "VISITOR-NEW" {
		t.Fatalf("calls %d", len(f.players))
	}
	if r.cachedVisitor(time.Now()) != "VISITOR-NEW" {
		t.Fatal("new visitor id not kept")
	}
	// Next time: one player call, at once with the page.
	if _, err := r.Look(context.Background(), vid); err != nil || len(f.players) != 3 {
		t.Fatalf("third call %v %d", err, len(f.players))
	}
}

// A watch page that fails costs the peak and the page's storyboard only.
func TestLookWithoutThePage(t *testing.T) {
	f := &fakeYouTube{play: okPlayer, page: func() (int, string) { return 500, "" }}
	v, err := f.serve(t).Look(context.Background(), vid)
	if err != nil || v.HLS == "" || v.PeakFound || v.Storyboard != "short-spec" {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestLookRefusesBadIDsAndHugeAnswers(t *testing.T) {
	f := &fakeYouTube{play: func(string) (int, string) { return 200, `{"x":"` + strings.Repeat("a", maxPlayer) + `"}` }}
	r := f.serve(t)
	for _, id := range []string{"", "short", "aqz-KE-bpKQ&x=1", "../../etc/pw"} {
		if ReasonOf(func() error { _, err := r.Look(context.Background(), id); return err }()) != BadID {
			t.Fatalf("%q accepted", id)
		}
	}
	if len(f.players)+len(f.pages) != 0 {
		t.Fatal("a bad id reached YouTube")
	}
	if _, err := r.Look(context.Background(), vid); ReasonOf(err) != Network {
		t.Fatalf("oversized answer: %v", err)
	}
}

func TestPeak(t *testing.T) {
	if _, ok := Peak(nil); ok {
		t.Fatal("no graph has no peak")
	}
	if p, ok := Peak([]Mark{{40, 1}}); !ok || p != 40 {
		t.Fatal("one part is the peak")
	}
	// Order does not matter; the opening (0 s, highest) is skipped.
	if p, _ := Peak([]Mark{{20, 0.5}, {0, 1}, {10, 0.7}, {30, 0.1}}); p != 10 {
		t.Fatalf("peak %v", p)
	}
	// The first 5% are skipped too: 100 parts of 1 s, high at 0-3 s and 50 s.
	var m []Mark
	for i := 0; i < 100; i++ {
		v := 0.1
		switch {
		case i < 4:
			v = 1 - float64(i)*0.01
		case i == 50:
			v = 0.8
		}
		m = append(m, Mark{float64(i), v})
	}
	if p, _ := Peak(m); p != 50 {
		t.Fatalf("peak %v", p)
	}
}

// Addresses skips the watch page while a visitor id is kept, reads it when
// none is, and reads it after all for a fresh id when YouTube refuses one.
func TestAddressesSkipsThePage(t *testing.T) {
	f := &fakeYouTube{play: func(v string) (int, string) {
		if v != "VISITOR-NEW" {
			return status("LOGIN_REQUIRED", "Sign in to confirm you're not a bot")(v)
		}
		return okPlayer(v)
	}}
	r := f.serve(t)
	// No visitor yet: the page is needed for one.
	v, err := r.Addresses(context.Background(), vid)
	if err != nil || v.HLS == "" || len(f.pages) != 1 || !v.Page {
		t.Fatalf("first %v pages %d page %v", err, len(f.pages), v.Page)
	}
	// Kept: the player call alone, so no peak and no page.
	v, err = r.Addresses(context.Background(), vid)
	if err != nil || v.HLS == "" || len(f.pages) != 1 || v.Page || v.PeakFound || len(f.players) != 2 {
		t.Fatalf("second %v pages %d players %d %+v", err, len(f.pages), len(f.players), v)
	}
	// A refused id: the page after all, then the player call once more.
	r.keepVisitor("VISITOR-OLD")
	if v, err = r.Addresses(context.Background(), vid); err != nil || v.HLS == "" || len(f.pages) != 2 || len(f.players) != 4 {
		t.Fatalf("renewal %v pages %d players %d", err, len(f.pages), len(f.players))
	}
}

func TestLookSaysWhetherThePageWasRead(t *testing.T) {
	f := &fakeYouTube{play: okPlayer}
	if v, err := f.serve(t).Look(context.Background(), vid); err != nil || !v.Page {
		t.Fatalf("page read: %v %v", v.Page, err)
	}
	f = &fakeYouTube{play: okPlayer, page: func() (int, string) { return 503, "" }}
	if v, err := f.serve(t).Look(context.Background(), vid); err != nil || v.Page {
		t.Fatalf("failed page: %v %v", v.Page, err)
	}
}

// "Try again later" is YouTube holding the door back, not the video.
func TestTryAgainLaterIsNotTheVideo(t *testing.T) {
	f := &fakeYouTube{play: status("ERROR", "This content isn't available, try again later.")}
	_, err := f.serve(t).Look(context.Background(), vid)
	if ReasonOf(err) != Refused || Refused.VideoReason() {
		t.Fatal(err)
	}
}

// A peak in the last 10% (the closing card) leaves nothing to watch.
func TestPeakSkipsTheEnd(t *testing.T) {
	var m []Mark
	for i := 0; i < 100; i++ {
		v := 0.1
		switch i {
		case 0:
			v = 1
		case 30:
			v = 0.5
		case 95:
			v = 0.9
		}
		m = append(m, Mark{float64(i), v})
	}
	if p, _ := Peak(m); p != 30 {
		t.Fatalf("peak %v", p)
	}
}

// A watch page without the player part (YouTube's bot check withholds it)
// still gives the length, from the graph's parts.
func TestLengthFromTheGraph(t *testing.T) {
	page := watchHTML("VISITOR-NEW", 3)
	i := strings.Index(page, `<script>var ytInitialPlayerResponse`)
	j := strings.Index(page[i:], `</script>`) + i + len(`</script>`)
	f := &fakeYouTube{play: status("LOGIN_REQUIRED", "Sign in to confirm you're not a bot"), page: func() (int, string) { return 200, page[:i] + page[j:] }}
	v, err := f.serve(t).Look(context.Background(), vid)
	if ReasonOf(err) != BotCheck || !v.Page || !v.PeakFound || v.Peak != 30 || v.Duration != 100 {
		t.Fatalf("%+v %v", v, err)
	}
}
