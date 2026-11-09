//go:build lab

package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestLabMaindata runs the change feed against the lab qBittorrent only, never a real client, behind a counting proxy. Set the key from labctl qbt-apikey in the environment first:
//
//	go test -tags lab -run LabMaindata -v ./internal/downloads/
//
// It adds a test torrent from the lab indexer and polls the feed while it moves. Every merged answer must equal torrents/info. It logs bytes per full and delta answer, then removes the torrent.
func TestLabMaindata(t *testing.T) {
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
		counts[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	defer front.Close()
	q := NewQbittorrentKey(front.URL, key, nil)
	ctx := context.Background()
	const testHash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	if _, err := q.Add(ctx, AddRequest{URL: "http://fake-indexer/dune-part-two.torrent", Category: "holyarrs-labtest",
		SavePath: "/data/downloads/incomplete/holyarrs-labtest", InfoHash: testHash}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := q.Remove(context.Background(), testHash, true); err != nil {
			t.Errorf("remove: %v", err)
		}
	}()
	var c Cursors
	st := &Store{}
	var full, deltas []int
	for i := range 20 {
		p, err := c.Poll(ctx, 1, 0, q)
		if err != nil {
			t.Fatal(err)
		}
		if p.Full {
			full = append(full, p.Bytes)
		} else {
			deltas = append(deltas, p.Bytes)
		}
		st.Update(1, 0, p.Jobs, time.Now())
		listed, err := q.ListAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if i%5 == 0 || i == 19 {
			if msg := parityDiff(p.Jobs, listed); msg != "" {
				t.Errorf("poll %d: %s", i, msg)
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	sort.Ints(deltas)
	t.Logf("MEASURE lab torrents=%d full_bytes=%v delta_bytes_min=%d median=%d max=%d requests=%v",
		len(st.Current(1).Items), full, deltas[0], deltas[len(deltas)/2], deltas[len(deltas)-1], counts)
	if len(full) != 1 {
		t.Errorf("%d full answers in 20 polls with a valid rid", len(full))
	}
	if counts["GET /api/v2/sync/maindata"] != 20 {
		t.Errorf("maindata requests %d, want one per poll", counts["GET /api/v2/sync/maindata"])
	}
}

// parityDiff compares the change-feed rows with torrents/info, ignoring live counters that move between the two reads.
func parityDiff(feed, listed []Job) string {
	norm := func(js []Job) map[string]Job {
		m := map[string]Job{}
		for _, j := range js {
			j.Progress, j.Speed, j.UpSpeed, j.ETA, j.Seeds, j.Peers, j.SeedsTotal, j.PeersTotal = 0, 0, 0, 0, 0, 0, 0, 0
			j.LastActivity, j.TimeActive, j.SeedingTime, j.Uploaded, j.Ratio, j.SeenComplete = 0, 0, 0, 0, 0, 0
			j.State, j.Detail, j.DoneAt, j.Tracker, j.TrackerProblem = "", "", 0, "", ""
			m[j.ID] = j
		}
		return m
	}
	a, b := norm(feed), norm(listed)
	if !reflect.DeepEqual(a, b) {
		for k, v := range b {
			if !reflect.DeepEqual(a[k], v) {
				return "row " + k + " differs between the change feed and torrents/info"
			}
		}
		return "row sets differ"
	}
	return ""
}
