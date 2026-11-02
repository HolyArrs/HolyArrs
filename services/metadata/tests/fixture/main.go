package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxPadding = 8 * 1024 * 1024
const maxRoutes = 4096
const maxTimestamps = 20000

type fault struct {
	Route        string `json:"route"`
	Reset        bool   `json:"reset"`
	DelayMS      int    `json:"delay_ms"`
	Status       int    `json:"status"`
	RetryAfter   string `json:"retry_after"`
	Mode         string `json:"mode"`
	PayloadBytes int    `json:"payload_bytes"`
}

type routeStats struct {
	Count      int      `json:"count"`
	Timestamps []string `json:"timestamps"`
}

type fixture struct {
	mu            sync.Mutex
	faults        map[string]fault
	total         int
	routes        map[string]*routeStats
	timestamps    []string
	droppedRoutes int
}

func newFixture() *fixture {
	return &fixture{faults: make(map[string]fault), routes: make(map[string]*routeStats)}
}

func (f *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/control" {
		f.control(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/healthz" {
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}
	if r.URL.Path == "/stats" {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, map[string]interface{}{"total": f.total, "routes": f.routes, "timestamps": f.timestamps, "dropped_routes": f.droppedRoutes, "timestamps_truncated": f.total > maxTimestamps})
		return
	}
	body, ok := providerBody(r)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]string{"error": "unknown synthetic route"})
		return
	}
	f.mu.Lock()
	f.total++
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if len(f.timestamps) < maxTimestamps {
		f.timestamps = append(f.timestamps, now)
	}
	rs := f.routes[r.URL.Path]
	if rs == nil && len(f.routes) < maxRoutes {
		rs = &routeStats{}
		f.routes[r.URL.Path] = rs
	}
	if rs != nil {
		rs.Count++
		if len(rs.Timestamps) < maxTimestamps {
			rs.Timestamps = append(rs.Timestamps, now)
		}
	} else {
		f.droppedRoutes++
	}
	active := f.faults[""]
	if specific, exists := f.faults[r.URL.Path]; exists {
		active = specific
	}
	f.mu.Unlock()
	if active.DelayMS > 0 {
		timer := time.NewTimer(time.Duration(active.DelayMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}
	if active.RetryAfter != "" {
		w.Header().Set("Retry-After", active.RetryAfter)
	}
	if active.Status != 0 && active.Status != 200 {
		w.WriteHeader(active.Status)
		writeJSON(w, map[string]string{"error": "synthetic fault"})
		return
	}
	if active.Mode == "incomplete" {
		body = map[string]interface{}{"synthetic": true}
	}
	size := active.PayloadBytes
	if active.Mode == "oversized" && size <= 2*1024*1024 {
		size = 2*1024*1024 + 1
	}
	if size > 0 {
		padding := strings.Repeat("x", size)
		switch b := body.(type) {
		case map[string]interface{}:
			if overview, exists := b["overview"].(string); exists {
				b["overview"] = overview + padding
			} else {
				b["synthetic_padding"] = padding
			}
		case []interface{}:
			if len(b) > 0 {
				b[0].(map[string]interface{})["synthetic_padding"] = padding
			}
		}
	}
	encoded, _ := json.Marshal(body)
	switch active.Mode {
	case "malformed":
		encoded = []byte(`{"synthetic":!}`)
	case "truncated":
		encoded = encoded[:len(encoded)/2]
	}
	_, _ = w.Write(encoded)
}

func (f *fixture) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var next fault
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&next) != nil || next.DelayMS < 0 || next.DelayMS > 120000 || next.PayloadBytes < 0 || next.PayloadBytes > maxPadding || (next.Status != 0 && (next.Status < 200 || next.Status > 599)) || strings.ContainsAny(next.Route, "?#\r\n") || len(next.RetryAfter) > 128 {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid control"})
		return
	}
	switch next.Mode {
	case "", "malformed", "truncated", "incomplete", "oversized":
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if next.Reset {
		f.faults = make(map[string]fault)
		f.routes = make(map[string]*routeStats)
		f.total = 0
		f.timestamps = nil
		f.droppedRoutes = 0
	}
	if next.Route == "*" {
		next.Route = ""
	}
	f.faults[next.Route] = next
	writeJSON(w, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, body interface{}) { _ = json.NewEncoder(w).Encode(body) }

var positive = regexp.MustCompile(`^[1-9][0-9]*$`)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func numericID(s string) (int, bool) {
	if !positive.MatchString(s) {
		return 0, false
	}
	n, e := strconv.Atoi(s)
	return n, e == nil && n > 0 && n <= 1000000000
}

func providerBody(r *http.Request) (interface{}, bool) {
	p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(p) >= 4 && p[0] == "tmdb" && p[1] == "3" {
		if len(p) == 4 && p[2] == "search" && (p[3] == "movie" || p[3] == "tv") {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			results := []interface{}{}
			if page <= 100 {
				results = append(results, tmdbDetail(p[3], page))
			}
			return map[string]interface{}{"page": page, "total_pages": 100, "total_results": 100, "results": results}, true
		}
		id, ok := numericID(p[3])
		if !ok {
			return nil, false
		}
		if len(p) == 4 && (p[2] == "movie" || p[2] == "tv") {
			return tmdbDetail(p[2], id), true
		}
		if len(p) == 6 && p[2] == "tv" && p[4] == "season" {
			season, err := strconv.Atoi(p[5])
			if err != nil || season < 0 || season > 1000 {
				return nil, false
			}
			return map[string]interface{}{"id": tmdbSeasonID(id, season), "name": fmt.Sprintf("Synthetic Season %d", season), "season_number": season, "air_date": "2020-01-02", "episodes": []interface{}{map[string]interface{}{"id": id*10001 + season + 1, "episode_number": 1, "season_number": season, "name": "Synthetic Épisode 一", "air_date": "2020-01-02", "runtime": 24}}}, true
		}
	}
	if len(p) >= 3 && p[0] == "tvmaze" {
		if len(p) == 3 && p[1] == "search" && p[2] == "shows" {
			return []interface{}{map[string]interface{}{"score": 1.0, "show": tvmazeDetail(1)}}, true
		}
		if len(p) == 3 && p[1] == "shows" {
			if id, ok := numericID(p[2]); ok {
				return tvmazeDetail(id), true
			}
		}
	}
	if (len(p) == 4 || len(p) == 5) && p[0] == "musicbrainz" && p[1] == "ws" && p[2] == "2" {
		plural := map[string]string{"artist": "artists", "release-group": "release-groups", "release": "releases", "recording": "recordings"}[p[3]]
		if plural == "" {
			return nil, false
		}
		if len(p) == 4 {
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			if offset < 0 {
				offset = 0
			}
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit < 1 {
				limit = 25
			}
			if limit > 100 {
				limit = 100
			}
			items := []interface{}{}
			for i := offset; i < offset+limit && i < 10000; i++ {
				items = append(items, musicDetail(p[3], fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)))
			}
			return map[string]interface{}{"count": 10000, "offset": offset, plural: items}, true
		}
		if uuid.MatchString(p[4]) {
			return musicDetail(p[3], p[4]), true
		}
	}
	return nil, false
}

func tmdbDetail(kind string, id int) map[string]interface{} {
	b := map[string]interface{}{"id": id, "overview": "Synthetic résumé 日本語", "original_language": "en", "genres": []interface{}{map[string]interface{}{"id": 1, "name": "Synthetic Genre"}}, "translations": map[string]interface{}{"translations": []interface{}{map[string]interface{}{"iso_639_1": "ja", "iso_3166_1": "JP", "data": map[string]interface{}{"title": "合成タイトル", "name": "合成シリーズ", "overview": "合成の概要"}}}}, "credits": map[string]interface{}{"cast": []interface{}{map[string]interface{}{"id": 1, "name": "Synthetic Person", "character": "Synthetic Character", "order": 0}}, "crew": []interface{}{map[string]interface{}{"id": 2, "name": "Synthetic Creator", "job": "Director"}}}}
	if kind == "movie" {
		b["title"] = fmt.Sprintf("Synthetic Movie %d", id)
		b["original_title"] = fmt.Sprintf("Synthetic Movie %d", id)
		b["release_date"] = "2020-01-02"
		b["runtime"] = 93
		b["release_dates"] = map[string]interface{}{"results": []interface{}{map[string]interface{}{"iso_3166_1": "US", "release_dates": []interface{}{map[string]interface{}{"type": 3, "release_date": "2020-01-02T00:00:00Z"}}}}}
	} else {
		b["name"] = fmt.Sprintf("Synthetic Show %d", id)
		b["original_name"] = fmt.Sprintf("Synthetic Show %d", id)
		b["first_air_date"] = "2020-01-02"
		b["last_air_date"] = "2020-02-02"
		b["number_of_seasons"] = 1
		b["number_of_episodes"] = 2
		b["status"] = "Ended"
		b["seasons"] = []interface{}{map[string]interface{}{"id": tmdbSeasonID(id, 0), "season_number": 0, "name": "Synthetic Specials", "episode_count": 1}, map[string]interface{}{"id": tmdbSeasonID(id, 1), "season_number": 1, "name": "Synthetic Season 1", "episode_count": 1}}
	}
	return b
}

func tmdbSeasonID(showID, season int) int {
	return showID*1001 + season + 1
}

func tvmazeDetail(id int) map[string]interface{} {
	return map[string]interface{}{"id": id, "name": fmt.Sprintf("Synthetic Show %d", id), "summary": "<p>Synthetic résumé 日本語</p>", "language": "English", "genres": []string{"Synthetic Genre"}, "premiered": "2020-01-02", "ended": "2020-02-02", "status": "Ended", "runtime": 24, "_embedded": map[string]interface{}{"episodes": []interface{}{map[string]interface{}{"id": id * 2, "name": "Synthetic Special", "season": 0, "number": 1, "airdate": "2020-01-01"}, map[string]interface{}{"id": id*2 + 1, "name": "Synthetic Épisode 一", "season": 1, "number": 1, "airdate": "2020-01-02"}}}}
}

func musicDetail(kind, id string) map[string]interface{} {
	artist := map[string]interface{}{"id": "00000000-0000-4000-8000-000000000002", "name": "Synthetic Artist É 一", "sort-name": "Synthetic Artist"}
	credit := []interface{}{map[string]interface{}{"name": "Synthetic Artist É 一", "artist": artist}}
	group := map[string]interface{}{"id": "00000000-0000-4000-8000-000000000003", "title": "Synthetic Album", "primary-type": "Album", "first-release-date": "2020-01-02"}
	recording := map[string]interface{}{"id": "00000000-0000-4000-8000-000000000004", "title": "Synthetic Track 一", "length": 120000, "artist-credit": credit}
	b := map[string]interface{}{"id": id, "title": "Synthetic " + kind, "artist-credit": credit, "disambiguation": "Synthetic fixture only"}
	switch kind {
	case "artist":
		b["name"] = artist["name"]
		b["sort-name"] = "Synthetic Artist"
		b["type"] = "Group"
		b["country"] = "US"
		b["aliases"] = []interface{}{map[string]interface{}{"name": "合成アーティスト", "locale": "ja"}}
		b["release-groups"] = []interface{}{group}
	case "release-group":
		b["primary-type"] = "Album"
		b["first-release-date"] = "2020-01-02"
		b["releases"] = []interface{}{map[string]interface{}{"id": "00000000-0000-4000-8000-000000000005", "title": "Synthetic Edition", "date": "2020-01-02", "country": "US", "status": "Official"}}
	case "release":
		b["date"] = "2020-01-02"
		b["country"] = "US"
		b["status"] = "Official"
		b["release-group"] = group
		b["media"] = []interface{}{map[string]interface{}{"position": 1, "format": "CD", "title": "Synthetic Disc", "track-count": 1, "tracks": []interface{}{map[string]interface{}{"id": "00000000-0000-4000-8000-000000000006", "number": "1", "position": 1, "title": "Synthetic Track 一", "length": 120000, "recording": recording}}}}
	case "recording":
		b["length"] = 120000
		b["releases"] = []interface{}{map[string]interface{}{"id": "00000000-0000-4000-8000-000000000005", "title": "Synthetic Edition", "release-group": group}}
		b["relations"] = []interface{}{map[string]interface{}{"type": "performance", "target-type": "work", "work": map[string]interface{}{"id": "00000000-0000-4000-8000-000000000007", "title": "Synthetic Work"}}}
	}
	return b
}

func main() {
	listen := flag.String("listen", ":8081", "private synthetic fixture listen address")
	flag.Parse()
	server := &http.Server{Addr: *listen, Handler: newFixture(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 130 * time.Second, IdleTimeout: 30 * time.Second}
	log.Print("synthetic fixture starting")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("synthetic fixture stopped")
	}
}
