package alerts

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Every channel can be added with no switch set: none is held back.
func TestEveryChannelWithNoSwitchSet(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	s, v := open(t)
	ctx := context.Background()
	for _, in := range []Input{
		{Name: "Hook", Kind: "webhook", URL: "https://hooks.example.com/in", Secret: "hk-1234"},
		{Name: "TG", Kind: "telegram", BotToken: tgToken, ChatID: "42"},
		{Name: "N", Kind: "ntfy", Topic: "holyarrs"},
		{Name: "G", Kind: "gotify", ServerURL: "https://gotify.example.com", AppToken: gotifyToken},
		{Name: "P", Kind: "pushover", AppToken: pushToken, UserKey: pushUser},
	} {
		if _, err := Add(ctx, s, v, in, domain.System); err != nil {
			t.Errorf("%s: %v", in.Kind, err)
		}
	}
}

// A finished download and a failed one are moments of their own: a new
// Alert speaks for both, and a webhook gets each under its name, with the
// movie and the Release.
func TestDownloadMoments(t *testing.T) {
	for _, m := range []string{MomentDownloaded, MomentDownloadFailed} {
		if !slices.Contains(Moments, m) {
			t.Fatalf("%s is not a moment: %v", m, Moments)
		}
	}
	if MomentOf(domain.Event{Kind: "download_vanished"}) != "" || MomentOf(domain.Event{Kind: "download_downloading"}) != "" {
		t.Fatal("only a finished or failed download is a moment")
	}
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeHook(t)
	a, err := Add(ctx, s, v, Input{Name: "Hook", Kind: "webhook", URL: srv.URL + "/in", Secret: "hk-1234", AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(a.Moments, MomentDownloaded) || !slices.Contains(a.Moments, MomentDownloadFailed) {
		t.Fatalf("a new Alert speaks for %v", a.Moments)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid := movieWorld(t, s)
	for i, c := range []struct{ kind, moment, subject, summary string }{
		{"download_finished", MomentDownloaded, "Downloaded: Arrival (2016)", "Download abcd1234: finished"},
		{"download_failed", MomentDownloadFailed, "Download failed: Arrival (2016)", "Download failed: no progress for 6 hours"},
	} {
		e := domain.Event{Kind: c.kind, TitleID: &tid, Summary: c.summary, Data: map[string]any{"state": strings.TrimPrefix(c.kind, "download_"), "progress": 1.0,
			"save_path": "/downloads/complete/Arrival.2016.1080p", "release": "Arrival.2016.1080p.WEB", "release_id": int64(7), "downloader": "qBit"}}
		m, ok := snd.MessageFor(ctx, e)
		if !ok || m.Moment != c.moment || m.Subject != c.subject || !strings.Contains(m.Text, "release: Arrival.2016.1080p.WEB") {
			t.Fatalf("%s: message %+v", c.kind, m)
		}
		if err := snd.Send(ctx, m); err != nil {
			t.Fatal(err)
		}
		g := got()
		if len(g) != i+1 || g[i].header.Get(HeaderEvent) != c.moment {
			t.Fatalf("%s: %d posts, event %q", c.kind, len(g), g[len(g)-1].header.Get(HeaderEvent))
		}
		var p Payload
		if err := json.Unmarshal(g[i].body, &p); err != nil {
			t.Fatal(err)
		}
		if p.Moment != c.moment || p.Kind != c.kind || p.Movie == nil || p.Movie.Name != "Arrival" || p.Data["release"] != "Arrival.2016.1080p.WEB" ||
			p.Data["save_path"] != "Arrival.2016.1080p" || strings.Contains(string(g[i].body), "/downloads") {
			t.Fatalf("%s: payload %s", c.kind, g[i].body)
		}
	}
}

// The importer's Event holds Go values (lists of steps and results), not
// decoded JSON: their full paths are cut to the last name too.
func TestWebhookCutsPathsInEventValues(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	srv, got := fakeHook(t)
	if _, err := Add(ctx, s, v, Input{Name: "Hook", Kind: "webhook", URL: srv.URL + "/in", AllowLocal: &yes}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}
	tid := movieWorld(t, s)
	src := "/downloads/complete/Arrival.2016.1080p/Arrival.2016.1080p.mkv"
	e := domain.Event{Kind: "import_done", TitleID: &tid, Summary: "Import done: 1 placed by hardlink", Data: map[string]any{
		"import_id": int64(3), "from": "/downloads/complete/Arrival.2016.1080p",
		"paths":  []map[string]string{{"from": src, "to": "/movies/Arrival (2016)/Arrival (2016).mkv", "method": "hardlink"}},
		"result": []mediafiles.PlaceResult{{SourceFile: src, Outcome: "placed", PlacedBy: "hardlink", HeldAside: "/movies/.holding/old.mkv"}}}}
	m, ok := snd.MessageFor(ctx, e)
	if !ok {
		t.Fatal("no message")
	}
	if err := snd.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	body := string(got()[0].body)
	for _, full := range []string{"/downloads", "/movies", "holding"} {
		if strings.Contains(body, full) {
			t.Fatalf("a full path left this machine (%s): %s", full, body)
		}
	}
	if !strings.Contains(body, `"source_file":"Arrival.2016.1080p.mkv"`) || !strings.Contains(body, `"to":"Arrival (2016).mkv"`) || !strings.Contains(body, `"import_id":3`) {
		t.Fatalf("payload %s", body)
	}
}
