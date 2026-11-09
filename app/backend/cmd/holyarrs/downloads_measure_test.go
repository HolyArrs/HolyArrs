package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestDownloadsViewRefetchCost(t *testing.T) {
	t.Parallel()
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
