package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/hooks"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// TestPluginSubtitleSiteEndToEnd: a fake subtitle-site plugin, polling over
// HTTP with its token, offers a subtitle and then makes it. Its offers are
// machine-made whatever it says, it learns where the video is only as its
// own folder and the path inside it, and a plugin without
// sources.subtitles gets 403.
func TestPluginSubtitleSiteEndToEnd(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	reg := newPluginHooks()
	old := sites.PluginSite
	t.Cleanup(func() { sites.PluginSite = old })
	wireSubtitleSites(reg.Broker, func(_ context.Context, plugin string, fileID int64) (hooks.SubtitleFile, error) {
		if plugin == "ws" && fileID == 7 {
			return hooks.SubtitleFile{FileID: 7, Folder: "movies", Path: "Arrival (2016)/Arrival.mkv"}, nil
		}
		return hooks.SubtitleFile{FileID: fileID}, nil
	})
	au := accounts.New(st, accounts.Options{Vault: v})
	a := buildHandlers(services{Store: st, Queue: jobs.New(st, jobs.Options{Workers: 1}), Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v,
		Live: webserver.NewLiveHub(webserver.LiveOptions{}), PluginHooks: reg})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	full := accounts.IssuePluginToken("ws", []string{"sources.subtitles"})
	none := accounts.IssuePluginToken("reader", []string{"sources.releases"})
	t.Cleanup(func() { accounts.RevokePluginToken("ws"); accounts.RevokePluginToken("reader"); reg.Drop("ws") })
	p := stubPlugin{t: t, base: srv.URL, tok: full}
	q := stubPlugin{t: t, base: srv.URL, tok: none}
	for _, m := range []string{"GET", "POST"} {
		if code, _ := q.do(m, "/api/v1/plugin-hooks/subtitles", map[string]any{"job": "x"}); code != http.StatusForbidden {
			t.Fatalf("%s subtitles without its scope: %d", m, code)
		}
	}
	site, err := sites.New("plugin", sites.Config{URL: "plugin://ws"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := site.Search(ctx, sites.Query{FileID: 7}); !errors.Is(err, sites.ErrPluginGone) {
		t.Fatalf("before the plugin polled: %v", err)
	}

	var mu sync.Mutex
	var asks []hooks.SubtitleAsk
	var makes []hooks.SubtitleMake
	p.serve(ctx, "subtitles", func(j hooks.Job) any {
		mu.Lock()
		defer mu.Unlock()
		switch j.Op {
		case "search":
			var in hooks.SubtitleAsk
			_ = json.Unmarshal(j.Payload, &in)
			asks = append(asks, in)
			return []hooks.SubtitleOffer{{Key: "audio-1", Language: "en", Format: "srt"}, {Key: "", Language: "de"}}
		case "make":
			var in hooks.SubtitleMake
			_ = json.Unmarshal(j.Payload, &in)
			makes = append(makes, in)
			return hooks.SubtitleMade{Format: "srt", Body: []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")}
		}
		return nil
	})
	waitProvider(t, reg, hooks.Subtitles, "ws")

	cands, err := site.Search(ctx, sites.Query{Kind: "movie", Title: "Arrival", Year: 2016, Languages: []string{"en"}, FileID: 7,
		File: "/srv/media/movies/Arrival (2016)/Arrival.mkv", Hash: "0123456789abcdef"})
	if err != nil || len(cands) != 1 {
		t.Fatalf("search: %+v %v", cands, err)
	}
	c := cands[0]
	if c.Key != "audio-1" || c.MachineMade != sites.Transcribed || c.Link != strconv.Itoa(7) {
		t.Fatalf("candidate %+v", c)
	}
	mu.Lock()
	got := asks[0]
	mu.Unlock()
	if got.FileID != 7 || got.Folder != "movies" || got.Path != "Arrival (2016)/Arrival.mkv" || got.Title != "Arrival" || len(got.Languages) != 1 {
		t.Fatalf("plugin got %+v", got)
	}
	raw, _ := json.Marshal(got)
	if s := string(raw); strings.Contains(s, "/srv/media") || strings.Contains(s, "0123456789abcdef") {
		t.Fatalf("the full path or the hash reached the plugin: %s", s)
	}

	body, err := site.Download(ctx, c)
	if err != nil || string(body) != "1\n00:00:01,000 --> 00:00:02,000\nHello\n" {
		t.Fatalf("make: %q %v", body, err)
	}
	mu.Lock()
	m := makes[0]
	mu.Unlock()
	if m.Key != "audio-1" || m.FileID != 7 || m.Folder != "movies" {
		t.Fatalf("plugin got %+v", m)
	}
	// A link HolyArrs did not set is never followed.
	if _, err := site.Download(ctx, sites.Candidate{Key: "audio-1", Link: "/etc/passwd"}); err == nil {
		t.Fatal("a made-up link was used")
	}
	// The plugin stops: the site says so, and a make Job would try again.
	reg.Drop("ws")
	accounts.RevokePluginToken("ws")
	if _, err := site.Download(ctx, c); !errors.Is(err, sites.ErrPluginGone) {
		t.Fatalf("after stop: %v", err)
	}
}

// Before the plugins start, a plugin site says the plugin is not running,
// so a make Job waits instead of sending a video without its folder.
func TestSubtitleFileFor_NoEngine(t *testing.T) {
	none := func() *plugins.Engine { return nil }
	if _, err := subtitleFileFor(none, nil)(context.Background(), "ws", 9); !errors.Is(err, sites.ErrPluginGone) {
		t.Fatalf("%v", err)
	}
	// Wired before the queue starts: a make the restart put back finds a
	// plugin site at once, and it says the plugin is not running yet.
	old, oldBC := sites.PluginSite, sources.BotCheck
	t.Cleanup(func() { sites.PluginSite, sources.BotCheck = old, oldBC })
	sites.PluginSite = nil
	wireSourcePlugins(hooks.NewBroker(), none, nil)
	site, err := sites.New("plugin", sites.Config{URL: "plugin://ws"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := site.Download(context.Background(), sites.Candidate{Key: "k", Link: "9"}); !errors.Is(err, sites.ErrPluginGone) {
		t.Fatalf("make before the plugins start: %v", err)
	}
}
