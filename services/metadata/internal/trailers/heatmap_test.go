package trailers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/youtube"
)

// fakeYouTube answers the watch page (with a "most replayed" graph of the
// given parts, none: no graph) and the player call (status OK or not).
func fakeYouTube(t *testing.T, marks []float64, status string) *youtube.Reader {
	t.Helper()
	return fakeYouTubeWith(t, marks, status, `{"hlsManifestUrl":"https://manifest.googlevideo.com/api/manifest/hls_variant/x/index.m3u8"}`)
}

// fakeYouTubeWith is fakeYouTube with the player's streaming data given.
func fakeYouTubeWith(t *testing.T, marks []float64, status, streaming string) *youtube.Reader {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/watch":
			var ms []string
			for i, v := range marks {
				ms = append(ms, fmt.Sprintf(`{"startMillis":"%d","intensityScoreNormalized":%v}`, i*20000, v))
			}
			io.WriteString(w, `<script>ytcfg.set({"VISITOR_DATA":"V1"});</script>`+
				`<script>var ytInitialData = {"m":{"markersList":{"markerType":"MARKER_TYPE_HEATMAP","markers":[`+strings.Join(ms, ",")+`]}}};</script>`)
		case "/youtubei/v1/player":
			io.WriteString(w, `{"playabilityStatus":{"status":"`+status+`","reason":"Sign in to confirm your age"},"videoDetails":{"lengthSeconds":"200"},
"streamingData":`+streaming+`}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &youtube.Reader{Base: srv.URL, HTTP: srv.Client()}
}

// The most replayed start comes from HolyArrs's own reader, the opening
// (always the highest part) skipped.
func TestHeatmapFromTheReader(t *testing.T) {
	r := Replays{Reader: fakeYouTube(t, []float64{1, 0.2, 0.3, 0.9, 0.4, 0.2, 0.1, 0.1, 0.1, 0.1}, "OK")}
	p, d, ok, err := r.Heatmap(context.Background(), CanaryKey)
	if err != nil || !ok || p != 60 || d != 200 {
		t.Fatal(p, d, ok, err)
	}
	r = Replays{Reader: fakeYouTube(t, nil, "OK")}
	if _, d, ok, err := r.Heatmap(context.Background(), CanaryKey); err != nil || ok || d != 200 {
		t.Fatal("no graph must be not found", d, ok, err)
	}
}

// A failure keeps the reader's reason, so a caller can tell one video's
// trouble (no pause) from the door's.
func TestHeatmapKeepsTheReason(t *testing.T) {
	r := Replays{Reader: fakeYouTube(t, []float64{1, 0.5}, "LOGIN_REQUIRED")}
	_, _, _, err := r.Heatmap(context.Background(), CanaryKey)
	if youtube.ReasonOf(err) != youtube.AgeRestricted || !youtube.ReasonOf(err).VideoReason() {
		t.Fatalf("reason of %v", err)
	}
	if _, _, _, err := r.Heatmap(context.Background(), "bad"); youtube.ReasonOf(err) != youtube.BadID {
		t.Fatalf("bad id: %v", err)
	}
}

// The server plays nothing: a video with only SABR formats still gives its
// most replayed start from the watch page. With no graph and no storyboard
// the failure stays, since the door may have changed.
func TestHeatmapWithoutAnAddress(t *testing.T) {
	sabr := `{"serverAbrStreamingUrl":"https://rr1.googlevideo.com/videoplayback?sabr=1"}`
	r := Replays{Reader: fakeYouTubeWith(t, []float64{1, 0.2, 0.3, 0.9, 0.4, 0.2, 0.1, 0.1, 0.1, 0.1}, "OK", sabr)}
	p, d, ok, err := r.Heatmap(context.Background(), CanaryKey)
	if err != nil || !ok || p != 60 || d != 200 {
		t.Fatal(p, d, ok, err)
	}
	r = Replays{Reader: fakeYouTubeWith(t, nil, "OK", sabr)}
	if _, _, _, err := r.Heatmap(context.Background(), CanaryKey); youtube.ReasonOf(err) != youtube.NoAddress {
		t.Fatalf("no graph, no storyboard and no address must stay a failure: %v", err)
	}
	// A storyboard alone is enough for the storyboard check.
	board := sabr + `,"storyboards":{"playerStoryboardSpecRenderer":{"spec":"https://i.ytimg.com/sb/x/storyboard3_L$L/$N.jpg|48#27#100#10#10#0#default#rs$x"}}`
	r = Replays{Reader: fakeYouTubeWith(t, nil, "OK", board)}
	if v, err := r.Look(context.Background(), CanaryKey); err != nil || v.Storyboard == "" || v.Duration != 200 {
		t.Fatal(v, err)
	}
}

// A watch page that could not be read is a failure, not "no graph", so the
// server stores nothing and reads the video again later.
func TestHeatmapUnreadPageIsAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/watch" {
			w.WriteHeader(503)
			return
		}
		io.WriteString(w, `{"playabilityStatus":{"status":"OK"},"videoDetails":{"lengthSeconds":"200"},
"streamingData":{"hlsManifestUrl":"https://manifest.googlevideo.com/api/manifest/hls_variant/x"}}`)
	}))
	t.Cleanup(srv.Close)
	r := Replays{Reader: &youtube.Reader{Base: srv.URL, HTTP: srv.Client()}}
	if _, _, ok, err := r.Heatmap(context.Background(), CanaryKey); ok || youtube.ReasonOf(err) != youtube.Network {
		t.Fatal(ok, err)
	}
}

// YouTube's bot check on the player call alone (seen from the server's
// address) keeps the watch page's most replayed start; the length comes
// from the graph. With no graph, the bot check stays a failure.
func TestHeatmapKeepsThePageOnABotCheck(t *testing.T) {
	serve := func(marks []float64) *youtube.Reader {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/watch" {
				var ms []string
				for i, v := range marks {
					ms = append(ms, fmt.Sprintf(`{"startMillis":"%d","intensityScoreNormalized":%v}`, i*20000, v))
				}
				io.WriteString(w, `<script>ytcfg.set({"VISITOR_DATA":"V1"});</script>`+
					`<script>var ytInitialData = {"m":{"markersList":{"markerType":"MARKER_TYPE_HEATMAP","markers":[`+strings.Join(ms, ",")+`]}}};</script>`)
				return
			}
			io.WriteString(w, `{"playabilityStatus":{"status":"LOGIN_REQUIRED","reason":"Sign in to confirm you're not a bot"}}`)
		}))
		t.Cleanup(srv.Close)
		return &youtube.Reader{Base: srv.URL, HTTP: srv.Client()}
	}
	r := Replays{Reader: serve([]float64{1, 0.2, 0.3, 0.9, 0.4, 0.2, 0.1, 0.1, 0.1, 0.1})}
	p, d, ok, err := r.Heatmap(context.Background(), CanaryKey)
	if err != nil || !ok || p != 60 || d != 200 {
		t.Fatal(p, d, ok, err)
	}
	r = Replays{Reader: serve(nil)}
	if _, _, _, err := r.Heatmap(context.Background(), CanaryKey); youtube.ReasonOf(err) != youtube.BotCheck {
		t.Fatalf("no graph must stay a bot check: %v", err)
	}
}
