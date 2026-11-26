package alerts

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Library moments reach the tap (the plugins) even with no Alert set up,
// and the cover art moment is one of them.
func TestTapGetsLibraryMomentsWithoutAlerts(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	var mu sync.Mutex
	var got []string
	snd := &Sender{Store: s, Vault: v, Backoff: []time.Duration{}, Tap: func(m Message) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, m.Moment)
	}}
	snd.Watch(ctx)
	tid := movieWorld(t, s)
	for _, k := range []string{domain.EventTitleAdded, domain.EventArtworkChanged, domain.EventAskMade, "search_done"} {
		if err := s.AddEvent(ctx, &domain.Event{Kind: k, TitleID: &tid, Summary: k}); err != nil {
			t.Fatal(err)
		}
	}
	snd.Wait()
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(slices.Sorted(slices.Values(got)), ",") != "add,artwork" {
		t.Fatalf("tapped %v", got)
	}
}

func TestArtworkMoment(t *testing.T) {
	if MomentOf(domain.Event{Kind: domain.EventArtworkChanged}) != MomentArtwork || !slices.Contains(Moments, MomentArtwork) {
		t.Fatal("cover art changes are not a moment")
	}
	for lang := range words {
		if say(lang, MomentArtwork) == MomentArtwork {
			t.Fatalf("%s: no words for the cover art moment", lang)
		}
	}
}

// A plugin's payload is the webhook's, without the names of the people who
// asked, and with full paths cut like a webhook's.
func TestPluginPayloadHasNoPeople(t *testing.T) {
	e := domain.Event{Kind: domain.EventTitleAdded, At: time.Now(), Data: map[string]any{"from": "/downloads/complete/Arrival"}}
	m := Message{Moment: MomentAdd, Subject: "Added: Arrival", People: []Person{{Name: "Ana", Email: "ana@a.test", DiscordID: "1"}}, Event: &e}
	p := PluginPayload(m)
	b, _ := json.Marshal(p)
	if p.People != nil || strings.Contains(string(b), "Ana") || strings.Contains(string(b), "ana@") {
		t.Fatalf("people in plugin payload: %s", b)
	}
	if p.Data["from"] != "Arrival" || p.Kind != domain.EventTitleAdded || p.Subject != "Added: Arrival" {
		t.Fatalf("payload %s", b)
	}
}

// Links (an indexer link with its key) and full paths never reach a plugin,
// in the text or the data; a problem report is never passed on.
func TestPluginPayloadHasNoLinks(t *testing.T) {
	e := domain.Event{Kind: domain.EventHealthBroken, Data: map[string]any{"message": "GET https://idx.test/api?apikey=SECRET failed", "list": []any{"http://u:p@dl.test/x"}}}
	m := Message{Moment: MomentHealth, Subject: "Health", Text: "Indexer https://idx.test/api?apikey=SECRET down\nfolder /data/media/Movies gone", Event: &e, Report: &Report{ID: 1, Note: "private"}}
	b, _ := json.Marshal(PluginPayload(m))
	for _, bad := range []string{"SECRET", "u:p@", "/data/media", "private"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("%q reached a plugin: %s", bad, b)
		}
	}
	if !strings.Contains(string(b), `down\nfolder Movies gone`) {
		t.Fatalf("text lost its lines or words: %s", b)
	}
}

// Keys and logins without a scheme (an indexer error quoting its query,
// user:pass@host, a nested people key) never reach a plugin either.
func TestPluginPayloadHasNoBareSecrets(t *testing.T) {
	e := domain.Event{Kind: "search_failed", Data: map[string]any{"error": "401 for idx.test/api?t=search&apikey=SECRET1", "inner": map[string]any{"user": "ana", "msg": "token=SECRET2"}}}
	m := Message{Moment: "", Text: "login u:SECRET3@nzb.test refused; passkey=SECRET4 bad", Event: &e}
	b, _ := json.Marshal(PluginPayload(m))
	for _, bad := range []string{"SECRET1", "SECRET2", "SECRET3", "SECRET4", "ana"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("%q reached a plugin: %s", bad, b)
		}
	}
	if !strings.Contains(string(b), "refused;") {
		t.Fatalf("text lost its words: %s", b)
	}
}

// Events beyond the Alerts moments reach plugins too; a request names its
// title and ids only, never who asked or what they wrote.
func TestWatchPluginsOnlyEvents(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	var mu sync.Mutex
	got := map[string]Payload{}
	snd := &Sender{Store: s, Vault: v}
	snd.WatchPlugins(ctx, func(ev string, _ int64, p Payload) { mu.Lock(); got[ev] = p; mu.Unlock() })
	tid := movieWorld(t, s)
	for _, e := range []*domain.Event{
		{Kind: domain.EventAskMade, TitleID: &tid, Summary: "Ana asked for Arrival", Data: map[string]any{"ask_id": 7, "tmdb_id": 329865, "asked_by": "Ana", "asked_by_ids": []int64{3}}},
		{Kind: domain.EventReportMade, TitleID: &tid, Summary: "Ana reported a problem", Data: map[string]any{"report_id": 2, "kind": "sound", "note": "my secret note", "made_by": "Ana"}},
		{Kind: domain.EventFileMissing, TitleID: &tid, Summary: "Missing /data/media/Movies/Arrival.mkv", Data: map[string]any{"path": "/data/media/Movies/Arrival.mkv"}},
		{Kind: domain.EventTitleAdded, TitleID: &tid, Summary: "not a plugin-only event"},
	} {
		if err := s.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	snd.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("events %v", got)
	}
	for ev, p := range got {
		b, _ := json.Marshal(p)
		for _, bad := range []string{"Ana", "secret", "/data/media", "asked_by"} {
			if strings.Contains(string(b), bad) {
				t.Fatalf("%s carries %q: %s", ev, bad, b)
			}
		}
		if p.Movie == nil || p.Movie.Name != "Arrival" {
			t.Fatalf("%s without its title: %s", ev, b)
		}
	}
	if got["request.made"].Data["ask_id"] == nil || got["report.made"].Data["kind"] != "sound" || got["library.file_missing"].Data["path"] != "Arrival.mkv" {
		t.Fatalf("ids lost: %+v", got)
	}
}

// Every kind a moment is caught up from is that moment, and every staff
// moment can be caught up.
func TestMomentKindsMatchMomentOf(t *testing.T) {
	for m, kinds := range momentKinds {
		for _, k := range kinds {
			if got := MomentOf(domain.Event{Kind: k}); got != m {
				t.Fatalf("%s is %q, listed under %q", k, got, m)
			}
		}
	}
	for m := range PluginMomentEvents {
		if len(momentKinds[m]) == 0 {
			t.Fatalf("moment %s has no kinds to catch up from", m)
		}
	}
}

// A stopped plugin catches up on the events it asked for, oldest first,
// with their ids, and nothing else.
func TestPluginReplay(t *testing.T) {
	s, v := open(t)
	ctx := context.Background()
	snd := &Sender{Store: s, Vault: v}
	tid := movieWorld(t, s)
	var ids []int64
	for _, k := range []string{domain.EventTitleAdded, domain.EventTitleRemoved, domain.EventAskMade, domain.EventTitleAdded} {
		e := &domain.Event{Kind: k, TitleID: &tid, Summary: "Added Arrival", Data: map[string]any{"asked_by": "Ana", "ask_id": 4}}
		if k == domain.EventAskMade {
			e.Summary = "Ana asked for Arrival"
		}
		if err := s.AddEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	got := snd.PluginReplay(ctx, []string{"library.add", "request.made"}, ids[0], 10)
	if len(got) != 2 || got[0].Event != "request.made" || got[0].ID != ids[2] || got[1].Event != "library.add" || got[1].ID != ids[3] {
		t.Fatalf("replay %+v (ids %v)", got, ids)
	}
	if b, _ := json.Marshal(got); strings.Contains(string(b), "Ana") {
		t.Fatalf("names in replay: %s", b)
	}
	if last, err := s.LastEventID(ctx); err != nil || last != ids[3] {
		t.Fatalf("last %d %v", last, err)
	}
}
