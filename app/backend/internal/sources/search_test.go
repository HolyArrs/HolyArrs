package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const duneFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>
<item><title>Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS</title><pubDate>Sat, 10 Oct 2026 12:00:00 +0000</pubDate>
<enclosure url="http://src/dune.torrent" length="29588180" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="1"/><torznab:attr name="infohash" value="29be7bfa8759643f2d5dac9f94d516c3378696ed"/></item>
<item><title>Dune.Part.Two.2024.720p.HDTV.x264-OTHER</title><pubDate>Sat, 10 Oct 2026 11:00:00 +0000</pubDate>
<enclosure url="http://src/dune720.torrent" length="29588180" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="4"/></item>
<item><title>Dune.Part.Two.2024.HDCAM.x264-BAD</title><pubDate>Sat, 10 Oct 2026 10:00:00 +0000</pubDate>
<enclosure url="http://src/cam.torrent" length="29588180" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="40"/></item>
<item><title>Arcane.S01E01.1080p.WEB.H264-HOLYARRS</title><pubDate>Sat, 10 Oct 2026 12:00:00 +0000</pubDate>
<enclosure url="http://src/arcane.torrent" length="206545177" type="application/x-bittorrent"/></item>
</channel></rss>`

type fixture struct {
	s      *database.Store
	slotID int64
	srcID  int64
}

func ptr[T any](v T) *T { return &v }

// seed adds a movie title with one item, Version and wanted Slot, and one Source.
func seed(t *testing.T, srcURL string) fixture {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var f fixture
	f.s = s
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune: Part Two", SortName: "dune part two", Year: ptr(2024),
			Aliases: []string{}, SourceIDs: map[string]string{"imdb": "tt15239678", "tmdb": "693134"}, MetadataSource: "local",
			KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand",
			Artwork: []string{}, Status: "released", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Dune Part Two (2024)", KeepUpToDate: true, IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotWanted}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Test", Protocol: "torrent", Format: "torznab", URL: srcURL, AllowLocal: true,
			UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"},
			Categories: map[string][]int{"movie": {2000}}, Priority: 25, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		f.slotID, f.srcID = sl.ID, src.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type pubs struct {
	mu     sync.Mutex
	topics []string
}

func (p *pubs) Publish(topic string, _ any) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.topics = append(p.topics, topic)
	return 0
}

func runSearch(t *testing.T, f fixture, sr *Searcher) domain.Job {
	t.Helper()
	q := jobs.New(f.s, jobs.Options{Workers: 1, BaseBackoff: time.Hour, PollInterval: 5 * time.Millisecond})
	q.Handle(jobs.KindSearchSlot, sr.Handle)
	ctx := context.Background()
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	id, _, err := q.Enqueue(ctx, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var j domain.Job
		if err := database.Get(ctx, f.s.Read(), "jobs", id, &j); err != nil {
			t.Fatal(err)
		}
		if j.State != "queued" && j.State != "running" || (j.State == "queued" && j.Attempt > 0) {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("search job did not finish")
	return domain.Job{}
}

func feedServer(t *testing.T, body string) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(file(t, "caps.xml"))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	return srv.URL + "/api"
}

func TestHandle_ChoosesWinner(t *testing.T) {
	f := seed(t, feedServer(t, duneFeed))
	p := &pubs{}
	var chosen []int64
	sr := &Searcher{Store: f.s, Live: p, OnChosen: func(_ context.Context, slotID, releaseID int64) { chosen = append(chosen, releaseID) }}
	j := runSearch(t, f, sr)
	if j.State != "done" {
		t.Fatalf("job %s: %+v", j.State, j.Error)
	}
	ctx := context.Background()
	var sl mediafiles.Slot
	if err := database.Get(ctx, f.s.Read(), "slots", f.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotChosen {
		t.Fatalf("slot state %s (%v)", sl.State, sl.LastReason)
	}
	rels, err := database.Select[domain.Release](ctx, f.s.Read(), "releases", "1=1 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 4 {
		t.Fatalf("stored %d releases, want 4", len(rels))
	}
	states := map[string]string{}
	for _, r := range rels {
		states[r.Name] = r.State
		if r.Verdict == nil || r.Verdict["reasons"] == nil {
			t.Errorf("%s has no verdict reasons", r.Name)
		}
		if r.JobID == nil || *r.JobID != j.ID || r.FoundBy != "auto_search" {
			t.Errorf("%s: job or found_by wrong", r.Name)
		}
	}
	want := map[string]string{
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS": "chosen",
		"Dune.Part.Two.2024.720p.HDTV.x264-OTHER":    "passed_over",
		"Dune.Part.Two.2024.HDCAM.x264-BAD":          "refused",
		"Arcane.S01E01.1080p.WEB.H264-HOLYARRS":      "refused",
	}
	for n, s := range want {
		if states[n] != s {
			t.Errorf("%s: state %s, want %s", n, states[n], s)
		}
	}
	if len(chosen) != 1 || chosen[0] != rels[0].ID {
		t.Fatalf("OnChosen got %v", chosen)
	}
	evs, _ := f.s.RecentEvents(ctx, 10)
	if len(evs) == 0 || !strings.Contains(evs[0].Summary, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS") {
		t.Fatalf("no search event naming the winner: %+v", evs)
	}
	if len(p.topics) == 0 {
		t.Fatal("nothing published live")
	}
}

func TestHandle_NothingPasses(t *testing.T) {
	f := seed(t, feedServer(t, `<rss><channel></channel></rss>`))
	j := runSearch(t, f, &Searcher{Store: f.s})
	if j.State != "done" {
		t.Fatalf("job %s", j.State)
	}
	var sl mediafiles.Slot
	_ = database.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl)
	if sl.State != mediafiles.SlotWanted || sl.LastReason == nil || !strings.Contains(*sl.LastReason, "no Release") {
		t.Fatalf("slot %s %v", sl.State, sl.LastReason)
	}
}

func TestHandle_PausesFailingSource(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	f := seed(t, srv.URL)
	j := runSearch(t, f, &Searcher{Store: f.s})
	if j.Error == nil {
		t.Fatalf("job should record an error, state %s", j.State)
	}
	var src domain.Source
	_ = database.Get(context.Background(), f.s.Read(), "sources", f.srcID, &src)
	if src.State != "paused" || src.PauseReason == nil || *src.PauseReason != KindDown || src.PausedUntil == nil || !src.PausedUntil.After(time.Now()) {
		t.Fatalf("source not paused: %+v", src)
	}
	if src.Extra["failures"] != "1" {
		t.Fatalf("failures %q", src.Extra["failures"])
	}
	var sl mediafiles.Slot
	_ = database.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl)
	if sl.State != mediafiles.SlotWanted {
		t.Fatalf("slot %s", sl.State)
	}
}

func TestCleanLink_KeepsSiteKeyHidesSourceKey(t *testing.T) {
	keys := []string{"mine77"}
	got := cleanLink("http://src/dl?id=5&apikey=site99&x=1&r=mine77", keys)
	if !strings.Contains(got, "apikey=site99") || strings.Contains(got, "mine77") {
		t.Fatalf("got %q", got)
	}
	back, err := revealLink(got, keys)
	if err != nil || back != "http://src/dl?id=5&apikey=site99&x=1&r=mine77" {
		t.Fatalf("back %q %v", back, err)
	}
}

func TestGrabLink_RSSPasskeyHiddenAndRestored(t *testing.T) {
	cfg := Config{Format: "rss"}
	cfg.APIKey = "pass" + "key=Abc%2Fdef123&uid=5"
	keys := secretValues(cfg)
	link := "http://tracker.example/dl/Abc%2Fdef123/42.torrent?passkey=Abc%2Fdef123&uid=5&raw=Abc/def123"
	got := cleanLink(link, keys)
	if strings.Contains(got, "def123") || !strings.Contains(got, "uid=5") {
		t.Fatalf("got %q", got)
	}
	back, err := revealLink(got, keys)
	if err != nil || back != link {
		t.Fatalf("back %q %v", back, err)
	}
	if _, err := revealLink(got, nil); err == nil {
		t.Fatal("a hidden link without its key should fail")
	}
}

func TestHandle_NeverStoresKeyAndGrabRestoresIt(t *testing.T) {
	key := "pk-" + "Zz9secret77"
	feed := strings.Replace(duneFeed, "http://src/dune.torrent", "http://src/dl/"+key+"/dune.torrent?passkey="+key+"&amp;apikey="+key, 1)
	f := seed(t, feedServer(t, feed))
	ctx := context.Background()
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE sources SET secret_ref = 7 WHERE id = ?", f.srcID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sec := fakeSecrets{7: key}
	j := runSearch(t, f, &Searcher{Store: f.s, Secrets: sec})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	rels, _ := database.Select[domain.Release](ctx, f.s.Read(), "releases", "1=1 ORDER BY id")
	var dune domain.Release
	for _, r := range rels {
		if strings.Contains(r.LinkRef, "Zz9secret77") {
			t.Fatalf("stored link holds the key: %q", r.LinkRef)
		}
		if strings.Contains(r.LinkRef, "dune.torrent") {
			dune = r
		}
	}
	evs, _ := f.s.RecentEvents(ctx, 50)
	for _, e := range evs {
		if b, _ := json.Marshal(e); strings.Contains(string(b), "Zz9secret77") {
			t.Fatalf("event holds the key: %s", b)
		}
	}
	got, err := GrabLink(ctx, f.s, sec, dune)
	want := "http://src/dl/" + key + "/dune.torrent?passkey=" + key + "&apikey=" + key
	if err != nil || got != want {
		t.Fatalf("grab link %q %v", got, err)
	}
}

type fakeSecrets map[int64]string

func (f fakeSecrets) Get(_ context.Context, id int64) (string, error) { return f[id], nil }

func TestClient_UsesSecretKey(t *testing.T) {
	sr := &Searcher{Secrets: fakeSecrets{7: "key-from-vault"}}
	ref := int64(7)
	c, err := sr.client(context.Background(), domain.Source{Meta: domain.Meta{ID: 1}, URL: "http://x/api", SecretRef: &ref})
	if err != nil || c.cfg.APIKey != "key-from-vault" {
		t.Fatalf("key %q %v", c.cfg.APIKey, err)
	}
	if c2, _ := sr.client(context.Background(), domain.Source{Meta: domain.Meta{ID: 1}, URL: "http://x/api"}); c2 == c || c2.cfg.APIKey != "" {
		t.Fatal("client kept the old key")
	}
}
