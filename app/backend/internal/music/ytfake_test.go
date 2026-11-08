package music

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// The fake YouTube the music video tests play against (a copy of the
// api trailer tests' fake).

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
	old := metadata.UseReader(&metadata.VideoReader{Base: srv.URL, HTTP: srv.Client()})
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

// trailerBatchView is the answer of GET /trailer/batch as the tests read it.
type trailerBatchView struct {
	Streams map[string]metadata.TrailerStream `json:"streams"`
	Videos  map[string]ArtistVideoView        `json:"videos"`
	Pending []string                          `json:"pending"`
}
