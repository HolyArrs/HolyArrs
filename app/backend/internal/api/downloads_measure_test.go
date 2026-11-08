//go:build measure

package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"runtime/metrics"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Option B refetch cost (downloads live plan, Phase 1): bytes and server
// time of one /downloads/view read holding 100, 500 and 2,000 rows, with 1
// and 8 tabs reading at once. Run:
//
//	go test -tags measure -run DownloadsViewRefetchCost -v ./internal/api/
func TestDownloadsViewRefetchCost(t *testing.T) {
	const notice = 2 * time.Second // B-hybrid: at most one "changed" notice per 2 s
	for _, rows := range []int{100, 500, 2000} {
		x := seed41(t, mediafiles.SlotDownloading, "downloading")
		own := min(rows, 100)
		err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
			var did int64
			if err := tx.QueryRow("SELECT downloader_id FROM downloads WHERE id = ?", x.dlID).Scan(&did); err != nil {
				return err
			}
			for i := range own - 1 {
				n := &domain.Download{DownloaderID: did, ReleaseID: &x.relID, SlotIDs: []int64{x.slotID}, ExternalID: fmt.Sprintf("%040x", 0xabc000+i),
					State: "downloading", Progress: 0.4, Speed: 1 << 20, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
				if _, err := database.Insert(tx, "downloads", n, domain.System); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		pointQB(t, x, qbList(t, realisticTorrents(rows-own)).URL)
		url := fmt.Sprintf("%s/api/v1/downloads/view?outside_limit=%d", x.srv.URL, rows)
		get := func() (int, time.Duration) {
			at := time.Now()
			resp, err := http.Get(url)
			if err != nil {
				t.Error(err)
				return 0, 0
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return len(b), time.Since(at)
		}
		get() // the first list read lands in the background
		time.Sleep(100 * time.Millisecond)
		size, _ := get()
		body := fetch(t, url)
		if n := strings.Count(body, `"id":`); n == 0 {
			t.Fatalf("no rows in %d bytes", len(body))
		}
		for _, tabs := range []int{1, 8} {
			const rounds = 15
			var lat []time.Duration
			var mu sync.Mutex
			cpu0 := cpuSeconds()
			wall := time.Now()
			for range rounds {
				var wg sync.WaitGroup
				for range tabs {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, d := get()
						mu.Lock()
						lat = append(lat, d)
						mu.Unlock()
					}()
				}
				wg.Wait()
			}
			cpu := cpuSeconds() - cpu0
			elapsed := time.Since(wall)
			sort.Slice(lat, func(i, k int) bool { return lat[i] < lat[k] })
			reads := float64(rounds * tabs)
			cpuPerRead := cpu / reads
			perTabBps := float64(size) / notice.Seconds()
			perTabCPU := cpuPerRead / notice.Seconds() * 100
			t.Logf("MEASURE rows=%d tabs=%d bytes=%d gzip=%d p50=%v p95=%v cpu_per_read_ms=%.2f per_tab_KBps=%.1f per_tab_cpu_pct=%.2f all_tabs_KBps=%.1f all_tabs_cpu_pct=%.2f wall=%v",
				rows, tabs, size, gzipLen(body), lat[len(lat)/2], lat[len(lat)*95/100], cpuPerRead*1000, perTabBps/1024, perTabCPU, perTabBps*float64(tabs)/1024, perTabCPU*float64(tabs), elapsed)
		}
	}
}

func fetch(t *testing.T, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func gzipLen(s string) int {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Len()
}

// cpuSeconds is the process's Go CPU time (user + GC), from runtime/metrics.
func cpuSeconds() float64 {
	s := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}, {Name: "/cpu/classes/idle:cpu-seconds"}}
	metrics.Read(s)
	return s[0].Value.Float64() - s[1].Value.Float64()
}

// realisticTorrents is n outside torrents with the fields a real client
// sends (long names, tracker, speeds, limits), 10% downloading.
func realisticTorrents(n int) string {
	parts := make([]string, n)
	for i := range n {
		state, prog, dl := "uploading", 1.0, 0
		if i%10 == 0 {
			state, prog, dl = "downloading", 0.37, 2_400_000
		}
		parts[i] = fmt.Sprintf(`{"hash":"%040x","name":"Some.Show.Name.S%02dE%02d.1080p.WEB-DL.DDP5.1.H.264-GROUP%d","category":"tv-sonarr","save_path":"/data/downloads/complete/tv","content_path":"/data/downloads/complete/tv/Some.Show.Name.S%02dE%02d","size":%d,"progress":%g,"dlspeed":%d,"upspeed":%d,"num_seeds":%d,"num_leechs":%d,"num_complete":120,"num_incomplete":14,"eta":%d,"state":"%s","added_on":%d,"ratio":1.73,"completion_on":%d,"priority":0,"tracker":"https://tracker.example.org:443/announce","trackers_count":2,"dl_limit":0,"up_limit":0,"last_activity":%d,"seeding_time":86400,"seen_complete":%d,"uploaded":%d,"time_active":90000,"ratio_limit":-2,"seeding_time_limit":-2,"inactive_seeding_time_limit":-2}`,
			i+1, i%30+1, i%24+1, i, i%30+1, i%24+1, 1_500_000_000+int64(i)*1000, prog, dl, 30_000+i, i%20, i%7, 8640000, state, 1700000000+i, 1700003600+i, 1730000000+i, 1730000000+i, 2_500_000_000+int64(i))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
