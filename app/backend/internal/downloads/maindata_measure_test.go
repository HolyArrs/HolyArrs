//go:build measure

package downloads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

type fakeMainServer struct {
	mu       sync.Mutex
	n        int
	active   int
	tick     int
	rid      int64
	torrents map[string]map[string]any
	changed  map[string]map[string]any
}

func (f *fakeMainServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/v2/torrents/info":
		list := make([]map[string]any, 0, len(f.torrents))
		for h, t := range f.torrents {
			c := map[string]any{"hash": h}
			for k, v := range t {
				c[k] = v
			}
			list = append(list, c)
		}
		_ = json.NewEncoder(w).Encode(list)
	case "/api/v2/sync/maindata":
		rid, _ := strconv.ParseInt(r.URL.Query().Get("rid"), 10, 64)
		out := map[string]any{"server_state": map[string]any{"dl_info_speed": 480_000_000, "up_info_speed": 6_000_000}}
		if rid == 0 || rid != f.rid {
			out["full_update"] = true
			out["torrents"] = f.torrents
		} else {
			out["torrents"] = f.changed
		}
		f.rid++
		out["rid"] = f.rid
		f.changed = map[string]map[string]any{}
		_ = json.NewEncoder(w).Encode(out)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestMaindataDeltaCost(t *testing.T) {
	for _, n := range []struct{ total, active int }{{50, 5}, {2000, 200}} {
		f := newFakeMain(n.total, n.active)
		srv := httptest.NewServer(f)
		q := NewQbittorrent(srv.URL, nil)
		ctx := context.Background()
		var st MainState
		d, err := q.SyncMain(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		at := time.Now()
		if _, err := st.Apply(d); err != nil {
			t.Fatal(err)
		}
		fullMerge := time.Since(at)
		full := d.Bytes
		listBytes := 0
		{
			resp, err := http.Get(srv.URL + "/api/v2/torrents/info")
			if err != nil {
				t.Fatal(err)
			}
			var raw json.RawMessage
			_ = json.NewDecoder(resp.Body).Decode(&raw)
			resp.Body.Close()
			listBytes = len(raw)
		}
		listAt := time.Now()
		if _, err := q.ListAll(ctx); err != nil {
			t.Fatal(err)
		}
		listParse := time.Since(listAt)
		var deltaBytes, deltaMax int
		var merge, mergeMax time.Duration
		const ticks = 20
		for range ticks {
			f.step()
			d, err := q.SyncMain(ctx, st.Rid)
			if err != nil || d.FullUpdate {
				t.Fatalf("delta %v full=%v", err, d.FullUpdate)
			}
			at := time.Now()
			if _, err := st.Apply(d); err != nil {
				t.Fatal(err)
			}
			_ = st.Jobs("")
			m := time.Since(at)
			merge += m
			mergeMax = max(mergeMax, m)
			deltaBytes += d.Bytes
			deltaMax = max(deltaMax, d.Bytes)
		}
		srv.Close()
		t.Logf("MEASURE torrents=%d active=%d full_bytes=%d full_merge=%v list_bytes=%d list_round_trip=%v delta_avg_bytes=%d delta_max_bytes=%d merge_avg=%v merge_max=%v delta_KBps_at_1.5s=%.1f list_KBps_at_1.5s=%.1f",
			n.total, n.active, full, fullMerge, listBytes, listParse, deltaBytes/ticks, deltaMax, merge/ticks, mergeMax, float64(deltaBytes/ticks)/1.5/1024, float64(listBytes)/1.5/1024)
	}
}

func newFakeMain(n, active int) *fakeMainServer {
	f := &fakeMainServer{n: n, active: active, torrents: map[string]map[string]any{}, changed: map[string]map[string]any{}}
	for i := range n {
		h := fmt.Sprintf("%040x", i+1)
		state, prog := "stalledUP", 1.0
		if i < active {
			state, prog = "downloading", 0.1
		}
		f.torrents[h] = map[string]any{
			"name": fmt.Sprintf("Some.Show.Name.S%02dE%02d.1080p.WEB-DL.DDP5.1.H.264-GROUP%d", i%30+1, i%24+1, i), "category": "tv-sonarr",
			"save_path": "/data/downloads/complete/tv", "content_path": "/data/downloads/complete/tv/x" + strconv.Itoa(i), "download_path": "",
			"size": 1_500_000_000 + i*1000, "total_size": 1_500_000_000 + i*1000, "progress": prog, "dlspeed": 0, "upspeed": 0, "num_seeds": 0, "num_leechs": 0,
			"num_complete": 120, "num_incomplete": 14, "eta": 8640000, "state": state, "added_on": 1700000000 + i, "ratio": 1.73, "completion_on": 1700003600 + i,
			"priority": 0, "tracker": "https://tracker.example.org:443/announce", "trackers_count": 2, "dl_limit": 0, "up_limit": 0, "last_activity": 1730000000,
			"seeding_time": 86400, "seen_complete": 1730000000, "uploaded": 2_500_000_000, "downloaded": 1_500_000_000, "time_active": 90000,
			"ratio_limit": -2, "seeding_time_limit": -2, "inactive_seeding_time_limit": -2, "max_ratio": -1, "max_seeding_time": -1, "availability": -1,
			"amount_left": 0, "completed": 1_500_000_000, "auto_tmm": false, "force_start": false, "super_seeding": false, "seq_dl": false, "f_l_piece_prio": false,
			"magnet_uri": "magnet:?xt=urn:btih:" + h + "&dn=Some.Show", "tags": "", "infohash_v1": h, "infohash_v2": "", "popularity": 0, "private": false,
			"reannounce": 1200, "comment": "", "has_metadata": true, "isPrivate": false, "dl_limit_session": 0, "upspeed_session": 0,
		}
	}
	return f
}

func (f *fakeMainServer) step() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tick++
	set := func(h string, k string, v any) {
		f.torrents[h][k] = v
		if f.changed[h] == nil {
			f.changed[h] = map[string]any{}
		}
		f.changed[h][k] = v
	}
	for i := range f.n {
		h := fmt.Sprintf("%040x", i+1)
		if i < f.active {
			p := f.torrents[h]["progress"].(float64) + 0.001
			set(h, "progress", p)
			set(h, "dlspeed", 2_400_000+(f.tick*37+i)%50_000)
			set(h, "upspeed", 30_000+(f.tick*13+i)%9_000)
			set(h, "eta", 600-f.tick)
			set(h, "num_seeds", 10+(f.tick+i)%5)
			set(h, "num_leechs", 3+(f.tick+i)%4)
			set(h, "downloaded", int(p*1.5e9))
			set(h, "completed", int(p*1.5e9))
			set(h, "amount_left", int((1-p)*1.5e9))
			set(h, "time_active", 90000+f.tick)
			set(h, "last_activity", 1730000000+f.tick)
			set(h, "availability", 12.5)
		} else if i%10 == f.tick%10 {
			set(h, "upspeed", 5_000+i%1000)
			set(h, "uploaded", 2_500_000_000+f.tick*1000)
			set(h, "ratio", 1.73+float64(f.tick)/1e4)
		}
	}
}
