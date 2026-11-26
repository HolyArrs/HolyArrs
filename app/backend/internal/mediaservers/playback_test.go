package mediaservers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const demoMovieSession = `<Video type="movie" ratingKey="18" sessionKey="1" title="The Matrix" duration="60000" viewOffset="6000">
	<User id="1" title="Alex"/><Player title="TV" state="playing"/><Session id="s1"/>
	<Guid id="tmdb://603"/><Media><Part decision="directplay"/></Media>
</Video>`

const demoEpisodeSession = `<Video type="episode" ratingKey="19" sessionKey="2" title="The Signal" grandparentTitle="Deep Space" duration="54000">
	<User id="2" title="Alex"/><Player title="Tablet" state="playing"/><Session id="s2"/>
	<TranscodeSession videoDecision="transcode"/>
</Video>`

func readPlaybackForTest(t *testing.T, w *world, m domain.MediaServer, filter playbackFilter, now time.Time) playbackPage {
	t.Helper()
	view, err := readPlayback(t.Context(), w.s.Read(), m, filter, now)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestPlaybackPollsOnlyOptedInServers(t *testing.T) {
	w := newWorld(t)
	movie, _ := watchTitles(t, w)
	var requests atomic.Int32
	var unavailable atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Plex-Token") != tok || r.Method != "GET" || r.URL.Path != "/status/sessions" {
			http.Error(rw, "wrong request", http.StatusUnauthorized)
			return
		}
		if unavailable.Load() {
			http.Error(rw, "Plex offline", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(rw, `<MediaContainer size="2">`+demoMovieSession+demoEpisodeSession+`</MediaContainer>`)
	}))
	defer srv.Close()
	input := Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}
	ctx := t.Context()
	m, err := Add(ctx, w.s, w.v, input, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	if err := l.PollPlayback(ctx); err != nil || requests.Load() != 0 {
		t.Fatalf("off made %d requests, err %v", requests.Load(), err)
	}
	on := true
	input.MonitorPlayback, input.Token = &on, ""
	m, err = Change(ctx, w.s, w.v, m.ID, input, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.PollPlayback(ctx); err != nil || requests.Load() != 1 {
		t.Fatalf("on made %d requests, err %v", requests.Load(), err)
	}
	view := readPlaybackForTest(t, w, m, playbackFilter{}, time.Now())
	if len(view.Active) != 2 || view.Plays != 0 || view.Stale || view.LastSeenAt == nil {
		t.Fatalf("wrong live view: %+v", view)
	}
	for _, play := range view.Active {
		switch play.Title {
		case "The Matrix":
			if play.TitleID == nil || *play.TitleID != movie || play.Method != "direct_play" || play.PositionMS != 6000 {
				t.Fatalf("movie did not map: %+v", play)
			}
		case "Deep Space · The Signal":
			if play.TitleID != nil || play.Method != "transcode" {
				t.Fatalf("unmatched episode: %+v", play)
			}
		default:
			t.Fatalf("unexpected play: %+v", play)
		}
	}
	unavailable.Store(true)
	if err := l.PollPlayback(ctx); err != nil {
		t.Fatal(err)
	}
	view = readPlaybackForTest(t, w, m, playbackFilter{}, time.Now())
	if !view.Stale || len(view.Active) != 2 || view.Plays != 0 {
		t.Fatalf("failed snapshot changed live streams: %+v", view)
	}
}

func TestPlaybackHistorySurvivesResumeAndRestart(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	on := true
	input := Input{Name: "Plex", Kind: "plex", URL: "http://localhost:32400", Token: tok, MonitorPlayback: &on}
	m, err := Add(ctx, w.s, w.v, input, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	state, err := playbackStateFor(ctx, w.s.Read(), m.ID)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC)
	snapshot := func(n int, status string, seenBefore bool) {
		t.Helper()
		xml := `<MediaContainer size="0"/>`
		if status != "" {
			xml = `<MediaContainer size="1">` + strings.Replace(demoMovieSession, `state="playing"`, `state="`+status+`"`, 1) + `</MediaContainer>`
		}
		samples, err := parsePlayback([]byte(xml))
		if err != nil {
			t.Fatal(err)
		}
		at := started.Add(time.Duration(n) * time.Second)
		applied, err := savePlaybackSnapshot(ctx, w.s, m.ID, state.generation, samples, at, seenBefore)
		if err != nil || !applied {
			t.Fatalf("save snapshot: applied=%t err=%v", applied, err)
		}
	}
	snapshot(0, "playing", false)
	snapshot(5, "playing", true)
	snapshot(10, "paused", true)
	snapshot(15, "paused", true)
	snapshot(20, "playing", true)
	snapshot(25, "playing", true)
	snapshot(30, "playing", false)
	snapshot(35, "playing", true)
	snapshot(40, "", true)

	view := readPlaybackForTest(t, w, m, playbackFilter{account: "1", title: "matrix"}, started.Add(40*time.Second))
	if view.Plays != 1 || view.WatchMS != 15000 || len(view.History) != 1 || len(view.Active) != 0 || !view.History[0].Incomplete {
		t.Fatalf("resume or restart inflated or split the play: %+v", view)
	}
	if got := readPlaybackForTest(t, w, m, playbackFilter{account: "2"}, started.Add(40*time.Second)); got.Plays != 0 {
		t.Fatalf("a different account got the play: %+v", got)
	}

	off := false
	input.MonitorPlayback, input.Token = &off, ""
	m, err = Change(ctx, w.s, w.v, m.ID, input, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	late := map[playbackKey]playbackSample{{Session: "late"}: {}}
	if applied, err := savePlaybackSnapshot(ctx, w.s, m.ID, state.generation, late, time.Now(), true); err != nil || applied {
		t.Fatalf("disabled source wrote an in-flight snapshot: applied=%t err=%v", applied, err)
	}
	view = readPlaybackForTest(t, w, m, playbackFilter{}, time.Now())
	if view.Enabled || view.Plays != 1 {
		t.Fatalf("turning off did not preserve history: %+v", view)
	}
	input.MonitorPlayback = &on
	m, err = Change(ctx, w.s, w.v, m.ID, input, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := playbackServers(ctx, w.s.Read())
	if err != nil || len(sources) != 1 {
		t.Fatalf("monitored sources = %v, err = %v", sources, err)
	}

	mux := http.NewServeMux()
	(&Handlers{Store: w.s, Vault: w.v}).Routes(mux)
	owner := webserver.ContextWith(ctx, webserver.Caller{Role: webserver.RoleOwner, Via: "session"})
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequestWithContext(owner, "DELETE", fmt.Sprintf("/api/v1/media-servers/%d/playback", m.ID), nil))
	if res.Code != http.StatusNoContent {
		t.Fatalf("clear history = %d %s", res.Code, res.Body)
	}
	if applied, err := savePlaybackSnapshot(ctx, w.s, m.ID, sources[0].Generation, late, time.Now(), true); err != nil || applied {
		t.Fatalf("clear allowed an in-flight snapshot: applied=%t err=%v", applied, err)
	}
	view = readPlaybackForTest(t, w, m, playbackFilter{}, time.Now())
	if !view.Enabled || view.Plays != 0 || len(view.Active) != 0 || len(view.People) != 0 {
		t.Fatalf("clear did not erase playback data: %+v", view)
	}
}

func TestPlaybackRequiresOwnerSession(t *testing.T) {
	w := newWorld(t)
	h := Handlers{Store: w.s, Vault: w.v}
	m, err := Add(t.Context(), w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: "http://localhost:32400", Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Routes(mux)
	path := "/api/v1/media-servers/" + strconv.FormatInt(m.ID, 10)
	for _, attempt := range []struct {
		method string
		suffix string
		body   string
	}{
		{"GET", "/playback", ""},
		{"DELETE", "/playback", ""},
		{"PUT", "", `{"name":"Plex","monitor_playback":true}`},
	} {
		for _, caller := range []webserver.Caller{
			{Role: webserver.RoleMember, Via: "session"},
			{Role: webserver.RoleAdmin, Via: "session"},
			{Role: webserver.RoleOwner, Via: "api_key", Scopes: []string{webserver.ScopeAdmin}},
		} {
			ctx := webserver.ContextWith(context.Background(), caller)
			r := httptest.NewRequestWithContext(ctx, attempt.method, path+attempt.suffix, strings.NewReader(attempt.body))
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, r)
			if res.Code != http.StatusForbidden {
				t.Fatalf("%s as %s via %s got %d", attempt.method, caller.Role, caller.Via, res.Code)
			}
		}
	}
}
