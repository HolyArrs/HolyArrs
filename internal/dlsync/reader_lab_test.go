//go:build lab

package dlsync

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// The reader and the Manager together against the lab qBittorrent only
// (never a real client), behind a counting proxy (the lab request counter):
//
//	python research\lab\labctl.py qbt-apikey -- go test -tags lab -run LabReader -v ./internal/dlsync/
//
// Each run adds our own Dune test torrent from the lab's fake indexer as a
// HolyArrs Download and times grab to finished (or to the first applied
// state, see target), with downloads.sync legacy
// and on (AC12 on the lab), then removes it. With on, the Manager must not
// list the client at all.
func TestLabReaderGrabToFinished(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_QBIT_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_QBIT_KEY not set (run through labctl qbt-apikey)")
	}
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://127.0.0.1:18708"
	}
	target, _ := url.Parse(base)
	var mu sync.Mutex
	counts := map[string]int{}
	proxy := httputil.NewSingleHostReverseProxy(target)
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[strings.TrimPrefix(r.URL.Path, "/api/v2/")]++
		mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	defer front.Close()
	q := downloader.NewQbittorrentKey(front.URL, key, nil)
	const testHash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	fastTimers(t)

	// target: "finished" times grab to finished (needs the lab seeder to
	// reach the client); HOLYARRS_LAB_TARGET=downloading times grab to the
	// Manager's first applied state when it does not.
	want := os.Getenv("HOLYARRS_LAB_TARGET")
	if want == "" {
		want = "finished"
	}
	run := func(mode string) (time.Duration, int64, map[string]int) {
		_ = q.Remove(context.Background(), testHash, true)
		time.Sleep(2 * time.Second)
		db, err := store.Open(t.TempDir(), store.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		err = db.Write(context.Background(), func(tx *sql.Tx) error {
			d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: front.URL, Category: "holyarrs-labtest", Priority: 1,
				PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
			_, err := store.Insert(tx, "downloaders", d, domain.System)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		r := &rig{db: db}
		r.m = &downloader.Manager{Store: db, HealthEvery: time.Hour, Engine: func(domain.Downloader) (downloader.Engine, error) { return q, nil }}
		r.m.OnList = func(int64, uint64, string, []downloader.Job, error) { r.ownLists.Add(1) }
		r.sh = &Shadow{Store: &Store{}, DB: db}
		r.s = &Sync{Store: r.sh.Store, DB: db, Mode: r.sh, Manager: r.m, Lines: testLines}
		r.m.Sync = r.s
		r.setMode(t, mode)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { r.s.Around(ctx, r.m.Run); close(done) }()
		time.Sleep(time.Second)
		mu.Lock()
		for k := range counts {
			delete(counts, k)
		}
		mu.Unlock()
		start := time.Now()
		if _, err := q.Add(context.Background(), downloader.AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs-labtest",
			SavePath: "/data/downloads/incomplete/holyarrs-labtest", InfoHash: testHash}); err != nil {
			t.Fatal(err)
		}
		err = db.Write(context.Background(), func(tx *sql.Tx) error {
			dl := &domain.Download{DownloaderID: 1, SlotIDs: []int64{}, ExternalID: testHash, State: "sent", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System, ProgressAt: &start}
			_, err := store.Insert(tx, "downloads", dl, domain.System)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		r.m.Poke()
		r.m.Kick(1)
		deadline := time.Now().Add(3 * time.Minute)
		for {
			var st string
			_ = db.Read().QueryRow("SELECT state FROM downloads WHERE external_id = ?", testHash).Scan(&st)
			if st == want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: still %q after 3 minutes", mode, st)
			}
			time.Sleep(20 * time.Millisecond)
		}
		took := time.Since(start)
		cancel()
		<-done
		mu.Lock()
		c := map[string]int{}
		for k, v := range counts {
			c[k] = v
		}
		mu.Unlock()
		return took, r.ownLists.Load(), c
	}
	var legacy, on []time.Duration
	for i := 0; i < 3; i++ {
		d, own, c := run(ModeLegacy)
		t.Logf("legacy run %d: %v, Manager lists %d, requests %v", i+1, d, own, c)
		legacy = append(legacy, d)
		d, own, c = run(ModeOn)
		t.Logf("on run %d: %v, Manager lists %d, requests %v", i+1, d, own, c)
		if own != 0 {
			t.Errorf("on: the Manager listed the client %d times", own)
		}
		if c["torrents/info"] > 2 {
			t.Errorf("on: %d torrents/info requests (only targeted reads expected)", c["torrents/info"])
		}
		on = append(on, d)
	}
	_ = q.Remove(context.Background(), testHash, true)
	sort.Slice(legacy, func(i, k int) bool { return legacy[i] < legacy[k] })
	sort.Slice(on, func(i, k int) bool { return on[i] < on[k] })
	t.Logf("legacy median %v max %v; on median %v max %v", legacy[1], legacy[2], on[1], on[2])
	if on[1] > legacy[1]+time.Second || on[2] > legacy[2]+2*time.Second {
		t.Errorf("on slower than legacy beyond AC12")
	}
}
