package source

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// item is one torznab result for the tests' own feeds.
func item(name string, pub time.Time, seeders int, hash string) string {
	h := ""
	if hash != "" {
		h = `<torznab:attr name="infohash" value="` + hash + `"/>`
	}
	return fmt.Sprintf(`<item><title>%s</title><pubDate>%s</pubDate><enclosure url="http://src/%s.torrent" length="4294967296" type="application/x-bittorrent"/>
<torznab:attr name="seeders" value="%d"/>%s</item>`, name, pub.Format(time.RFC1123Z), strings.ToLower(name), seeders, h)
}

func feedOf(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>` +
		strings.Join(items, "\n") + `</channel></rss>`
}

// liveFeed serves a body that can change between calls.
type liveFeed struct {
	mu   sync.Mutex
	body string
	hits int
}

func (l *liveFeed) set(b string) { l.mu.Lock(); l.body = b; l.mu.Unlock() }

func (l *liveFeed) url(t *testing.T) string {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(file(t, "caps.xml"))
			return
		}
		l.mu.Lock()
		l.hits++
		b := l.body
		l.mu.Unlock()
		_, _ = w.Write([]byte(b))
	})
	return srv.URL + "/api"
}

func exec(t *testing.T, f fixture, q string, args ...any) {
	t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// runKind runs one Job of kind through a real queue and waits for it.
func runKind(t *testing.T, f fixture, sr *Searcher, kind string, subj *domain.Subject, params map[string]any, setup ...func(*jobs.Queue)) domain.Job {
	t.Helper()
	q := jobs.New(f.s, jobs.Options{Workers: 1, BaseBackoff: time.Hour, PollInterval: 5 * time.Millisecond})
	sr.Queue = q
	sr.Register(q)
	for _, fn := range setup {
		fn(q)
	}
	ctx := context.Background()
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	id, _, err := q.Enqueue(ctx, kind, subj, params, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var j domain.Job
		if err := store.Get(ctx, f.s.Read(), "jobs", id, &j); err != nil {
			t.Fatal(err)
		}
		if j.State != "queued" && j.State != "running" || (j.State == "queued" && j.Attempt > 0) {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return domain.Job{}
}

func slotState(t *testing.T, f fixture) domain.Slot {
	t.Helper()
	var sl domain.Slot
	if err := store.Get(context.Background(), f.s.Read(), "slots", f.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	return sl
}

func relByName(t *testing.T, f fixture, name string) domain.Release {
	t.Helper()
	rs, err := store.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ? ORDER BY id DESC LIMIT 1", name)
	if err != nil || len(rs) != 1 {
		t.Fatalf("release %s: %v %d", name, err, len(rs))
	}
	return rs[0]
}

func feedOn(t *testing.T, f fixture) {
	exec(t, f, `UPDATE sources SET use_for = '{"feed":true,"auto_search":true,"hand_search":true}'`)
}

func waitMinutes(t *testing.T, f fixture, n int) {
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.protocol.wait_minutes_torrent', ?, '$.protocol.skip_wait_at_good_enough', json('false'))`, n)
}

const (
	web1080 = "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	tv720   = "Dune.Part.Two.2024.720p.HDTV.x264-OTHER"
	br1080  = "Dune.Part.Two.2024.1080p.BluRay.x264-GROUP"
)

// Roadmap 3.1.1: a feed hit grabs.
func TestFeedHitChooses(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, ""), item("Arcane.S01E01.1080p.WEB.H264-HOLYARRS", time.Now(), 5, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, _, rel int64) { chosen = append(chosen, rel) }}
	j := runKind(t, f, sr, KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	if sl := slotState(t, f); sl.State != domain.SlotChosen {
		t.Fatalf("slot %s %v", sl.State, sl.LastReason)
	}
	rel := relByName(t, f, web1080)
	if rel.FoundBy != "feed" || rel.State != "chosen" || len(chosen) != 1 || chosen[0] != rel.ID {
		t.Fatalf("release %+v chosen %v", rel, chosen)
	}
	// A Release for no Title in the Library is not kept.
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases WHERE name LIKE 'Arcane%'").Scan(&n)
	if n != 0 {
		t.Fatal("stored a Release that matches nothing")
	}
	// The next feed check does not look at the same Release again.
	exec(t, f, "UPDATE slots SET state = 'wanted'")
	runKind(t, f, sr, KindFeedCheck, nil, nil)
	if len(chosen) != 1 {
		t.Fatalf("the same feed Release was picked twice: %v", chosen)
	}
}

func TestFeedSkipsSourcesNotUsedForFeeds(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, lf.url(t))
	runKind(t, f, &Searcher{Store: f.s}, KindFeedCheck, nil, nil)
	if lf.hits != 0 || slotState(t, f).State != domain.SlotWanted {
		t.Fatalf("hits %d", lf.hits)
	}
}

// Roadmap 3.1.1: a Held Release is grabbed after its wait, and a better one
// arriving during the wait replaces it.
func TestHeldThenBetterThenWaitOver(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(tv720, time.Now(), 50, "")))
	f := seed(t, lf.url(t))
	feedOn(t, f)
	waitMinutes(t, f, 60)
	var chosen []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, _, rel int64) { chosen = append(chosen, rel) }}
	runKind(t, f, sr, KindFeedCheck, nil, nil)
	sl := slotState(t, f)
	first := relByName(t, f, tv720)
	if sl.State != domain.SlotHeld || sl.HeldReleaseID == nil || *sl.HeldReleaseID != first.ID || first.State != "held" || first.HoldUntil == nil {
		t.Fatalf("slot %s, release %s", sl.State, first.State)
	}
	// A better one shows up during the wait: it is held instead.
	lf.set(feedOf(item(tv720, time.Now(), 50, ""), item(web1080, time.Now(), 50, "")))
	runKind(t, f, sr, KindFeedCheck, nil, nil)
	sl = slotState(t, f)
	second := relByName(t, f, web1080)
	if sl.State != domain.SlotHeld || *sl.HeldReleaseID != second.ID {
		t.Fatalf("slot %s held %v, want %d", sl.State, sl.HeldReleaseID, second.ID)
	}
	if got := relByName(t, f, tv720); got.State != "passed_over" {
		t.Fatalf("first Held release is %s", got.State)
	}
	// Not over yet: nothing happens.
	runKind(t, f, sr, KindGrabHeld, nil, nil)
	if len(chosen) != 0 {
		t.Fatal("grabbed during the wait")
	}
	// The wait ends.
	exec(t, f, "UPDATE slots SET next_try_at = ?", store.FormatTime(time.Now().Add(-time.Minute)))
	j := runKind(t, f, sr, KindGrabHeld, nil, nil)
	if j.State != "done" || slotState(t, f).State != domain.SlotChosen || len(chosen) != 1 || chosen[0] != second.ID {
		t.Fatalf("after the wait: job %s slot %s chosen %v", j.State, slotState(t, f).State, chosen)
	}
	if got := relByName(t, f, web1080); got.State != "chosen" {
		t.Fatalf("held release %s", got.State)
	}
}

// Roadmap 3.1.1: a bulk search skips not-out-yet and downloading Slots,
// oldest searched first, and respects the count.
func TestMissingSearchSkipsNotOutYetAndBusy(t *testing.T) {
	f := seed(t, "http://unused/api")
	ctx := context.Background()
	// Three more Slots of the same Item kind: not out yet, downloading, and a second wanted.
	var extra []int64
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		for i, st := range []domain.SlotState{domain.SlotNotOutYet, domain.SlotDownloading, domain.SlotWanted} {
			var pr, lfID, tiID int64
			_ = tx.QueryRow("SELECT pick_rules_id, library_folder_id, title_id FROM versions LIMIT 1").Scan(&pr, &lfID, &tiID)
			it := &domain.Item{TitleID: tiID, Kind: "movie", KeepUpToDate: true}
			if _, err := store.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			v := &domain.Version{TitleID: tiID, Label: fmt.Sprintf("V%d", i), PickRulesID: pr, LibraryFolderID: lfID, FolderName: fmt.Sprintf("x%d", i), KeepUpToDate: true}
			if _, err := store.Insert(tx, "versions", v, domain.System); err != nil {
				return err
			}
			sl := &domain.Slot{ItemID: it.ID, VersionID: v.ID, State: st}
			if _, err := store.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			extra = append(extra, sl.ID)
		}
		// The fixture's Slot was searched lately; the new wanted one never.
		_, err := tx.Exec("UPDATE items SET last_searched_at = ? WHERE id = (SELECT item_id FROM slots WHERE id = ?)", store.FormatTime(time.Now()), f.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	exec(t, f, "UPDATE settings SET value = '1' WHERE grp = 'what_to_pick' AND key = 'missing_search_count'")
	sr := &Searcher{Store: f.s}
	j := runKind(t, f, sr, KindSearchMissing, nil, map[string]any{"scheduled": true})
	if j.State != "done" {
		t.Fatalf("job %s %v", j.State, j.Error)
	}
	got := queuedSearches(t, f)
	if len(got) != 1 || got[0] != extra[2] {
		t.Fatalf("queued %v, want only the never-searched wanted Slot %d", got, extra[2])
	}
	// A person's "search all missing" has no count but still skips them.
	exec(t, f, "DELETE FROM jobs WHERE kind = ?", KindSearchSlot)
	// The queued search ran (and failed on the unused Source): put it back.
	exec(t, f, "UPDATE slots SET state = 'wanted' WHERE id IN (?, ?)", f.slotID, extra[2])
	runKind(t, f, sr, KindSearchMissing, nil, map[string]any{"which": "missing"})
	got = queuedSearches(t, f)
	if len(got) != 2 || got[0] != extra[2] || got[1] != f.slotID {
		t.Fatalf("queued %v", got)
	}
}

func queuedSearches(t *testing.T, f fixture) []int64 {
	t.Helper()
	js, err := store.Select[domain.Job](context.Background(), f.s.Read(), "jobs", "kind = ? ORDER BY id", KindSearchSlot)
	if err != nil {
		t.Fatal(err)
	}
	var out []int64
	for _, j := range js {
		out = append(out, j.Subject.ID)
	}
	return out
}

// Roadmap 3.1.1: two Sources are merged and deduped by info hash or name.
func TestSearchMergesSourcesAndDedupes(t *testing.T) {
	a, b := &liveFeed{}, &liveFeed{}
	a.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), item(tv720, time.Now().Add(-time.Hour), 10, "")))
	b.set(feedOf(item("Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.265-RENAMED", time.Now().Add(-time.Hour), 10, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		item(tv720, time.Now().Add(-time.Hour), 10, ""), item(br1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, a.url(t))
	bURL := b.url(t)
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		src := &domain.Source{Name: "Second", Protocol: "torrent", Format: "torznab", URL: bURL, AllowLocal: true,
			UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"},
			Categories: map[string][]int{"movie": {2000}}, Priority: 30, State: "enabled", Extra: map[string]string{}}
		_, err := store.Insert(tx, "sources", src, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	j := runKind(t, f, &Searcher{Store: f.s}, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"choose": false})
	if j.State != "done" {
		t.Fatalf("job %s", j.State)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases").Scan(&n)
	if n != 3 {
		t.Fatalf("stored %d Releases, want 3 (same hash and same name kept once)", n)
	}
	if r := relByName(t, f, tv720); r.SourceID != f.srcID {
		t.Fatal("the copy from the higher-priority Source should stay")
	}
	// Showing only: nothing is chosen and the Slot does not move.
	if slotState(t, f).State != domain.SlotWanted || relByName(t, f, br1080).State != "judged" {
		t.Fatalf("show mode moved things: %s %s", slotState(t, f).State, relByName(t, f, br1080).State)
	}
}

// Picks per Source: a Source past its limit is skipped for the next best.
func TestPicksLimitPerSource(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item(web1080, time.Now().Add(-time.Hour), 10, "")))
	f := seed(t, lf.url(t))
	exec(t, f, `UPDATE sources SET limits = '{"grabs_per_window":1,"window_seconds":3600}'`)
	// One Download already came from this Source in the window.
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		r := &domain.Release{SourceID: f.srcID, Name: "Other", LinkRef: "x", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := store.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "c", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: d.ID, ReleaseID: &r.ID, SlotIDs: []int64{}, ExternalID: "e", State: "downloading", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := store.Insert(tx, "downloads", dl, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	runKind(t, f, &Searcher{Store: f.s}, KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, nil)
	got := relByName(t, f, web1080)
	if got.State != "passed_over" || !strings.Contains(fmt.Sprint(got.Verdict["reasons"]), "source_limit") || slotState(t, f).State != domain.SlotWanted {
		t.Fatalf("release %s slot %s", got.State, slotState(t, f).State)
	}
}
