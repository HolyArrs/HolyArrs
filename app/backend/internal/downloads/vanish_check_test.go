package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// askEngine is seedEngine whose Get can answer with a fixed error.
type askEngine struct {
	*seedEngine
	getErr error
	gets   int
}

func (a *askEngine) Get(ctx context.Context, id string) (Job, error) {
	a.gets++
	if a.getErr != nil {
		return Job{}, a.getErr
	}
	return a.seedEngine.Get(ctx, id)
}

// downEngine is a Downloader that does not answer.
type downEngine struct{ *seedEngine }

func (downEngine) List(context.Context, string) ([]Job, error) {
	return nil, errors.New("connection refused")
}

// onEngine grabs the rig's download, then swaps its Downloader for e with the
// given adapter, as a client other than qBittorrent.
func onEngine(t *testing.T, adapter string, e Engine) (*world, *Manager, int64) {
	t.Helper()
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := execSQL(w, "UPDATE downloaders SET adapter = '"+adapter+"'"); err != nil {
		t.Fatal(err)
	}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	m.Forget(w.download(t, id).DownloaderID)
	return w, m, id
}

func missAll(w *world, m *Manager, n int) {
	for range n {
		w.advance(missWindow)
		_, _ = m.SyncOnce(context.Background())
	}
}

func summaryOf(t *testing.T, w *world, kind string, id int64) string {
	t.Helper()
	var s string
	_ = w.s.ReadEvents().QueryRow("SELECT summary FROM events WHERE kind = ? AND download_id = ? ORDER BY id DESC LIMIT 1", kind, id).Scan(&s)
	return s
}

// Removed directly in the Downloader (scenarios 5, 6, 10, 18): vanished after
// the misses, with cleaned_at set, and History says "Removed outside HolyArrs",
// not "Download failed".
func TestVanishedReadsAsRemovedOutside(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.fake.drop(hash)
	missAll(w, m, missedSyncs)
	d := w.download(t, id)
	if d.State != "vanished" || d.CleanedAt == nil {
		t.Fatalf("download %s cleaned_at %v", d.State, d.CleanedAt)
	}
	s := summaryOf(t, w, "download_vanished", id)
	if !strings.HasPrefix(s, "Removed outside HolyArrs: ") || strings.Contains(s, "failed") {
		t.Fatalf("History summary %q", s)
	}
	if n := countEvents(t, w, "download_vanished", id); n != 1 {
		t.Fatalf("%d vanished events", n)
	}
	if r := w.release(t); !strings.HasPrefix(r.Verdict["turned_down_reason"].(string), "the download was removed outside HolyArrs") {
		t.Fatalf("turned down reason %v", r.Verdict)
	}
}

// A vanished download leaves the Downloads view and the raw list; its row
// stays for History.
func TestVanishedLeavesTheLists(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotFailed, "vanished")
	exec41(t, x.s, "UPDATE downloads SET cleaned_at = ? WHERE id = ?", database.FormatTime(time.Now()), x.dlID)
	pointQB(t, x, qbList(t, `[]`).URL)
	feedStore(t, x.dl)
	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); len(v.Downloads) != 0 {
		t.Fatalf("view rows: %+v", v.Downloads)
	}
	srv := httptest.NewServer(ListDownloads(x.s))
	t.Cleanup(srv.Close)
	if b := string(do(t, "GET", srv.URL, "", 200)); strings.Contains(b, `"vanished"`) {
		t.Fatalf("raw list: %s", b)
	}
}

// Re-added by hand after a vanish (scenario 12): a dead vanished row no
// longer claims the hash; after the short wait the job shows as an outside row.
func TestVanishedTorrentAddedAgainShowsAsOutside(t *testing.T) {
	for _, tc := range []struct {
		name string
		age  time.Duration
		want int
	}{{"inside the wait", time.Minute, 0}, {"after the wait", 11 * time.Minute, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			x := seed41(t, mediafiles.SlotFailed, "vanished")
			exec41(t, x.s, "UPDATE downloads SET cleaned_at = ? WHERE id = ?", database.FormatTime(time.Now().Add(-tc.age)), x.dlID)
			pointQB(t, x, qbList(t, `[{"hash":"ABCDABCDABCDABCDABCDABCDABCDABCDABCDABCD","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":0.3,"state":"downloading"}]`).URL)
			feedStore(t, x.dl)
			v := view(t, x.srv.URL+"/api/v1/downloads/view")
			if len(v.Downloads) != tc.want || v.OutsideTotal != tc.want {
				t.Fatalf("rows %d outside %d: %+v", len(v.Downloads), v.OutsideTotal, v.Downloads)
			}
			if tc.want == 1 && (!v.Downloads[0].ClientItem || !v.Downloads[0].Outside) {
				t.Fatalf("re-added job: %+v", v.Downloads[0])
			}
		})
	}
}

// Category changed in the client (scenario 9): the job still exists, so it is
// a hand-off: cleaned, not searched again, the Release kept, the Slot wanted,
// one History event.
func TestMovedOutOfCategoryIsNotDownloadedAgain(t *testing.T) {
	for _, tc := range []struct {
		name string
		eng  func() Engine
	}{
		{"found in another label", func() Engine {
			return &seedEngine{jobs: []Job{{ID: hash, Category: "keep", State: StateDownloading, Progress: 0.4}}}
		}},
		{"client says another category", func() Engine {
			return &askEngine{seedEngine: &seedEngine{}, getErr: errTransmissionForeign}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, m, id := onEngine(t, "transmission", tc.eng())
			missAll(w, m, missedSyncs)
			d := w.download(t, id)
			if d.State != "cleaned" || d.StateReason == nil || !strings.Contains(*d.StateReason, "moved out of HolyArrs's category") || d.CleanedAt == nil {
				t.Fatalf("download %s %v", d.State, d.StateReason)
			}
			if len(w.failedSlots) != 0 || w.release(t).State == "turned_down" {
				t.Fatalf("searched %v release %s", w.failedSlots, w.release(t).State)
			}
			if sl := w.slot(t); sl.State != mediafiles.SlotWanted {
				t.Fatalf("slot %s", sl.State)
			}
			if n := countEvents(t, w, "download_removed", id); n != 1 {
				t.Fatalf("%d removed events", n)
			}
			if s := summaryOf(t, w, "download_removed", id); !strings.HasPrefix(s, "Moved out of HolyArrs's category in ") {
				t.Fatalf("summary %q", s)
			}
		})
	}
}

// A job the client confirms is gone vanishes; an unclear answer to the one
// by-id question keeps following it.
func TestVanishAsksByIDOnce(t *testing.T) {
	eng := &askEngine{seedEngine: &seedEngine{}}
	w, m, id := onEngine(t, "deluge", eng)
	missAll(w, m, missedSyncs-1)
	if eng.gets != 0 {
		t.Fatalf("asked by id %d times before the misses were up", eng.gets)
	}
	missAll(w, m, 1)
	if d := w.download(t, id); d.State != "vanished" || eng.gets != 1 {
		t.Fatalf("download %s after %d gets", d.State, eng.gets)
	}

	odd := &askEngine{seedEngine: &seedEngine{}, getErr: errors.New("deluge: timeout")}
	w2, m2, id2 := onEngine(t, "deluge", odd)
	missAll(w2, m2, missedSyncs*3)
	if d := w2.download(t, id2); d.State == "vanished" || len(w2.failedSlots) != 0 {
		t.Fatalf("vanished on an unclear answer: %s", d.State)
	}
}

// A job the by-id question still finds in HolyArrs's category is followed on.
func TestVanishFoundByIDStays(t *testing.T) {
	eng := &askEngine{seedEngine: &seedEngine{jobs: []Job{{ID: hash, Category: "holyarrs", State: StateDownloading, Progress: 0.5}}}}
	// List leaves it out (a client glitch); Get finds it.
	w, m, id := onEngine(t, "rtorrent", &listless{eng})
	missAll(w, m, missedSyncs)
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("download %s", d.State)
	}
}

type listless struct{ *askEngine }

func (listless) List(context.Context, string) ([]Job, error) { return nil, nil }

// Downloader unreachable (scenario 8): nothing vanishes however long it lasts.
func TestUnreachableDownloaderNeverVanishes(t *testing.T) {
	w, m, id := onEngine(t, "transmission", downEngine{&seedEngine{}})
	missAll(w, m, 6)
	if d := w.download(t, id); d.State == "vanished" || len(w.failedSlots) != 0 {
		t.Fatalf("download %s searched %v", d.State, w.failedSlots)
	}
}

// Client restarted with an empty list (scenario 7): with more than one job
// followed there, nothing vanishes for the first hour; after that an empty
// list is a real removal.
func TestEmptyClientWaitsBeforeVanishing(t *testing.T) {
	w, m, id := onEngine(t, "qbittorrent", &seedEngine{})
	other := w.oldRow(t, w.download(t, id).DownloaderID, "1111111111111111111111111111111111111111", "downloading")
	missAll(w, m, 5) // 50 minutes
	if a, b := w.download(t, id), w.download(t, other); a.State == "vanished" || b.State == "vanished" {
		t.Fatalf("vanished inside the wait: %s %s", a.State, b.State)
	}
	missAll(w, m, missedSyncs+1)
	if a, b := w.download(t, id), w.download(t, other); a.State != "vanished" || b.State != "vanished" {
		t.Fatalf("still %s %s after the wait", a.State, b.State)
	}
}

// Seeding row removed in the client (scenario 14): once the import is done it
// becomes cleaned, with no failure and no search. One still listed stays.
func TestFinishedRemovedInClientIsCleaned(t *testing.T) {
	keep := Job{ID: "2222222222222222222222222222222222222222", Category: "holyarrs", State: StateDownloading, Progress: 0.1}
	eng := &seedEngine{jobs: []Job{keep}}
	w, m, id := onEngine(t, "qbittorrent", eng)
	for _, q := range []string{
		`UPDATE downloads SET state = 'finished', progress = 1`,
		`INSERT INTO imports (download_id, state, plan, from_path, kind, mode, created_at, created_by, updated_at, updated_by)
			VALUES (` + fmt.Sprint(id) + `, 'done', '[]', '/x', 'movie', 'auto', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`,
	} {
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
	}
	// An active download on the same Downloader: its list is read anyway.
	w.oldRow(t, w.download(t, id).DownloaderID, keep.ID, "downloading")
	missAll(w, m, missedSyncs-1)
	if d := w.download(t, id); d.State != "finished" {
		t.Fatalf("cleaned too early: %s", d.State)
	}
	missAll(w, m, 1)
	d := w.download(t, id)
	if d.State != "cleaned" || d.CleanedAt == nil || !strings.Contains(*d.StateReason, "removed outside HolyArrs") {
		t.Fatalf("download %s %v", d.State, d.StateReason)
	}
	if len(w.failedSlots) != 0 || w.release(t).State == "turned_down" || countEvents(t, w, "download_failed", id) != 0 {
		t.Fatalf("treated as a failure: searched %v release %s", w.failedSlots, w.release(t).State)
	}
	if n := countEvents(t, w, "download_removed", id); n != 1 {
		t.Fatalf("%d removed events", n)
	}
}

func TestFinishedStillListedStays(t *testing.T) {
	eng := &seedEngine{jobs: []Job{{ID: hash, Category: "holyarrs", State: StateFinished, Progress: 1}, {ID: "2222222222222222222222222222222222222222", Category: "holyarrs", State: StateDownloading}}}
	w, m, id := onEngine(t, "qbittorrent", eng)
	if err := execSQL(w, `UPDATE downloads SET state = 'finished', progress = 1`); err != nil {
		t.Fatal(err)
	}
	w.oldRow(t, w.download(t, id).DownloaderID, "2222222222222222222222222222222222222222", "downloading")
	missAll(w, m, missedSyncs*2)
	if d := w.download(t, id); d.State != "finished" {
		t.Fatalf("download %s", d.State)
	}
}

// The same hash on two Downloaders (scenario 19): one vanishing leaves the
// other's row alone.
func TestVanishIsPerDownloader(t *testing.T) {
	w, m, id := onEngine(t, "qbittorrent", &seedEngine{})
	other := w.addDownloader(t, &domain.Downloader{Name: "Second", Adapter: "transmission", Protocol: "torrent", URL: "http://second", Category: "holyarrs", State: "ok"})
	twin := w.oldRow(t, other, hash, "downloading")
	second := &seedEngine{jobs: []Job{{ID: hash, Category: "holyarrs", State: StateDownloading, Progress: 0.5}}}
	m.Engine = func(d domain.Downloader) (Engine, error) {
		if d.ID == other {
			return second, nil
		}
		return &seedEngine{}, nil
	}
	m.Forget(w.download(t, id).DownloaderID)
	m.Forget(other)
	missAll(w, m, missedSyncs)
	if a, b := w.download(t, id), w.download(t, twin); a.State != "vanished" || b.State == "vanished" || b.State == "cleaned" {
		t.Fatalf("first %s twin %s", a.State, b.State)
	}
}
