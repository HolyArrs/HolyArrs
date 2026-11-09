// Command explore-load is the Explore load test. It has three parts, all local and with fake data only:
//
//	fake -addr A [-delay D]     a stand-in TMDB (fake ids, no real calls)
//	seed -config DIR [-titles N -members N]  a fake Library and members, server stopped
//	run  -base URL -phase NAME [-dur D -members N]  load: GET explore head and tail
package main

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// passFile is where seed leaves the members' generated sign-in value; run
// reads it back. It is made fresh per config folder and never committed.
const passFile = "load-members.txt"

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: explore-load fake|seed|run ...")
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "fake":
		fake(args)
	case "seed":
		seed(args)
	case "run":
		run(args)
	default:
		log.Fatal("unknown command ", os.Args[1])
	}
}

var genreIDs = []int{28, 12, 16, 35, 80, 18, 27, 878, 53, 10749, 10751, 14}

func fake(args []string) {
	fs := flag.NewFlagSet("fake", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18744", "listen address")
	delay := fs.Duration("delay", 40*time.Millisecond, "time added to every answer")
	_ = fs.Parse(args)
	var calls atomic.Int64
	var bmu sync.Mutex
	byType := map[string]int64{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__stats" {
			bmu.Lock()
			b, _ := json.Marshal(map[string]any{"calls": calls.Load(), "by_type": byType})
			bmu.Unlock()
			_, _ = w.Write(b)
			return
		}
		if r.URL.Path == "/__reset" {
			bmu.Lock()
			calls.Store(0)
			byType = map[string]int64{}
			bmu.Unlock()
			return
		}
		calls.Add(1)
		bmu.Lock()
		byType[callType(r.URL.Path)]++
		bmu.Unlock()
		time.Sleep(*delay)
		switch {
		case strings.HasSuffix(r.URL.Path, "/genre/movie/list"), strings.HasSuffix(r.URL.Path, "/genre/tv/list"):
			_, _ = w.Write([]byte(`{"genres":[{"id":28,"name":"Action"},{"id":12,"name":"Adventure"},{"id":16,"name":"Animation"},{"id":35,"name":"Comedy"},{"id":80,"name":"Crime"},{"id":18,"name":"Drama"},{"id":27,"name":"Horror"},{"id":878,"name":"Science Fiction"},{"id":53,"name":"Thriller"},{"id":10749,"name":"Romance"}]}`))
		case strings.HasSuffix(r.URL.Path, "/search/person"):
			_, _ = w.Write([]byte(`{"results":[{"id":5}]}`))
		case strings.HasSuffix(r.URL.Path, "/combined_credits"):
			_, _ = w.Write([]byte(`{"cast":[]}`))
		case strings.Contains(r.URL.Path, "/v1/") || strings.Contains(r.URL.Path, "/images"):
			http.NotFound(w, r)
		default:
			h := fnv.New32a()
			h.Write([]byte(r.URL.String()))
			base := int(h.Sum32() % 6000)
			var parts []string
			for i := 0; i < 20; i++ {
				id := 1 + (base+i*37)%6000
				g := genreIDs[(id+i)%len(genreIDs)]
				parts = append(parts, fmt.Sprintf(`{"id":%d,"title":"Fake Movie %d","name":"Fake Show %d","release_date":"2026-%02d-10","first_air_date":"2026-%02d-10","genre_ids":[%d,%d],"popularity":%d,"vote_average":%.1f,"vote_count":%d,"poster_path":"/p%d.jpg","backdrop_path":"/b%d.jpg","overview":"Fake overview for title %d."}`,
					id, id, id, 1+id%10, 1+id%10, g, genreIDs[(id*7)%len(genreIDs)], 100-i, 5+float64(id%40)/10, 100+id%900, id, id, id))
			}
			_, _ = w.Write([]byte(`{"page":1,"results":[` + strings.Join(parts, ",") + `]}`))
		}
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// callType names the kind of outside call a fake TMDB path is.
func callType(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for len(parts) > 0 && parts[0] != "movie" && parts[0] != "tv" && parts[0] != "trending" && parts[0] != "discover" &&
		parts[0] != "person" && parts[0] != "search" && parts[0] != "genre" && parts[0] != "images" && parts[0] != "collection" && parts[0] != "find" {
		parts = parts[1:]
	}
	switch {
	case len(parts) == 0:
		return "other:" + p
	case parts[0] == "images":
		return "images"
	case parts[0] == "movie" || parts[0] == "tv":
		if len(parts) == 2 {
			if parts[1] == "upcoming" || parts[1] == "on_the_air" || parts[1] == "popular" || parts[1] == "now_playing" {
				return "list:" + parts[1]
			}
			return "details"
		}
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	return parts[0]
}

func seed(args []string) {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	dir := fs.String("config", "", "config folder (server stopped)")
	n := fs.Int("titles", 2000, "Library titles")
	members := fs.Int("members", 10, "members (the first is the owner)")
	asks := fs.String("asks", "0", "open asks (seeds) per member, comma list cycled over members, e.g. 5,12")
	_ = fs.Parse(args)
	if *dir == "" {
		log.Fatal("-config is required")
	}
	s, err := database.Open(*dir, database.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	rb := make([]byte, 12)
	if _, err := crand.Read(rb); err != nil {
		log.Fatal(err)
	}
	pass := hex.EncodeToString(rb)
	if err := os.WriteFile(filepath.Join(*dir, passFile), []byte(pass), 0o600); err != nil {
		log.Fatal(err)
	}
	hash, err := accounts.HashPassword(pass)
	if err != nil {
		log.Fatal(err)
	}
	err = s.WriteCore(ctx, func(tx *sql.Tx) error {
		for i := 1; i <= *members; i++ {
			role := "member"
			if i == 1 {
				role = "owner"
			}
			u := &domain.User{Name: fmt.Sprintf("member%02d", i), Role: role, PasswordHash: hash, State: "active", Perms: "[]"}
			if _, err := database.Insert(tx, "users", u, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("users: ", err)
	}
	rng := rand.New(rand.NewSource(7))
	allGenres := []string{"Action", "Adventure", "Animation", "Comedy", "Crime", "Drama", "Horror", "Science Fiction", "Thriller", "Romance"}
	ratings := []string{"G", "PG", "PG-13", "R"}
	now := time.Now().UTC()
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for i := 0; i < *n; i++ {
			kind := "movie"
			if i%4 == 3 {
				kind = "show"
			}
			year := 1980 + rng.Intn(46)
			g := []string{allGenres[rng.Intn(len(allGenres))], allGenres[rng.Intn(len(allGenres))]}
			ov := "Fake overview."
			t := &mediafiles.Title{Kind: kind, Name: fmt.Sprintf("Lab Title %04d", i), SortName: fmt.Sprintf("lab title %04d", i), Year: &year, Overview: &ov,
				Aliases: []string{}, SourceIDs: map[string]string{"tmdb": fmt.Sprint(1 + rng.Intn(6000))}, MetadataSource: "tmdb",
				KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, Artwork: []string{},
				Status: "released", State: "active", WhyAdded: "by_hand", GateDriver: "none", Genres: g, Rating: ratings[rng.Intn(len(ratings))],
				People: []string{fmt.Sprintf("Person %d", rng.Intn(300)), fmt.Sprintf("Person %d", rng.Intn(300))}, RuntimeMinutes: 90 + rng.Intn(60),
				MusicSecondary: []string{}, MusicStatuses: []string{}}
			if kind == "show" {
				t.Status = "running"
			}
			id, err := database.Insert(tx, "titles", t, domain.System)
			if err != nil {
				return fmt.Errorf("title %d: %w", i, err)
			}
			added := now.Add(-time.Duration(rng.Intn(400*24)) * time.Hour)
			if _, err := tx.Exec("UPDATE titles SET created_at = ? WHERE id = ?", database.FormatTime(added), id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("titles: ", err)
	}
	var perMember []int
	for _, f := range strings.Split(*asks, ",") {
		var v int
		fmt.Sscan(strings.TrimSpace(f), &v)
		perMember = append(perMember, v)
	}
	total := 0
	err = s.Write(ctx, func(tx *sql.Tx) error {
		for m := 1; m <= *members; m++ {
			k := perMember[(m-1)%len(perMember)]
			for i := 0; i < k; i++ {
				at := database.FormatTime(now.Add(-time.Duration(i+1) * 24 * time.Hour))
				kind := []string{"movie", "show"}[i%2]
				res, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, year, state, asked_by, asked_at)
					VALUES (?, '{}', ?, '{}', ?, ?, ?, ?, 'asked', ?, ?)`, at, at, kind, fmt.Sprint(7000+m*100+i), fmt.Sprintf("Asked %02d-%02d", m, i), 1990+(m*7+i)%35, m, at)
				if err != nil {
					return fmt.Errorf("ask: %w", err)
				}
				id, _ := res.LastInsertId()
				if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (?, ?, ?)", id, m, at); err != nil {
					return err
				}
				total++
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal("asks: ", err)
	}
	fmt.Printf("seeded %d titles, %d members and %d asks in %s\n", *n, *members, total, *dir)
}

type sample struct {
	kind     string
	dur      time.Duration
	bytes    int
	building bool
	status   int
}

func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	base := fs.String("base", "http://127.0.0.1:18743", "server")
	phase := fs.String("phase", "warm", "label")
	dur := fs.Duration("dur", 2*time.Minute, "run length")
	members := fs.Int("members", 10, "concurrent members")
	think := fs.Duration("think", 0, "pause after every request")
	out := fs.String("out", "", "write JSON results here")
	dir := fs.String("config", "", "config folder seed wrote the sign-in value to")
	_ = fs.Parse(args)
	raw, err := os.ReadFile(filepath.Join(*dir, passFile))
	if err != nil {
		log.Fatal("run needs -config of a seeded folder: ", err)
	}
	pass := strings.TrimSpace(string(raw))
	cls := make([]*http.Client, *members)
	for i := range cls {
		jar, _ := cookiejar.New(nil)
		cls[i] = &http.Client{Jar: jar, Timeout: 60 * time.Second}
		body := fmt.Sprintf(`{"name":"member%02d","password":"%s"}`, i+1, pass)
		resp, err := cls[i].Post(*base+"/api/v1/auth/login", "application/json", strings.NewReader(body))
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			log.Fatalf("login member%02d: %s", i+1, resp.Status)
		}
	}
	var mu sync.Mutex
	var samples []sample
	small := map[string]int{}
	builtAt := make([]time.Duration, *members)
	start := time.Now()
	deadline := start.Add(*dur)
	get := func(i int, kind, path string) (map[string]any, bool) {
		t0 := time.Now()
		resp, err := cls[i].Get(*base + path)
		if err != nil {
			mu.Lock()
			samples = append(samples, sample{kind: kind, dur: time.Since(t0), status: 0})
			mu.Unlock()
			return nil, false
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		d := time.Since(t0)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		bl, _ := m["building"].(bool)
		if len(b) < 300 && resp.StatusCode == 200 {
			mu.Lock()
			small[kind+" "+string(b)]++
			mu.Unlock()
		}
		mu.Lock()
		samples = append(samples, sample{kind: kind, dur: d, bytes: len(b), building: bl, status: resp.StatusCode})
		if kind == "head" && !bl && resp.StatusCode == 200 && builtAt[i] == 0 {
			builtAt[i] = time.Since(start)
		}
		mu.Unlock()
		time.Sleep(*think)
		return m, resp.StatusCode == 200
	}
	var wg sync.WaitGroup
	for i := 0; i < *members; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for time.Now().Before(deadline) {
				if _, ok := get(i, "head", "/api/v1/explore/head?type=all"); !ok {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				for page := 0; page < 12 && time.Now().Before(deadline); page++ {
					m, ok := get(i, "tail", fmt.Sprintf("/api/v1/explore/tail?type=all&page=%d", page))
					if !ok {
						break
					}
					if more, _ := m["more"].(bool); !more {
						break
					}
				}
			}
		}(i)
	}
	wg.Wait()
	res := map[string]any{"phase": *phase, "members": *members, "seconds": time.Since(start).Seconds()}
	groups := map[string][]sample{}
	for _, s := range samples {
		groups[s.kind] = append(groups[s.kind], s)
		state := "pool ready"
		if s.building {
			state = "pool building"
		}
		groups[s.kind+" ("+state+")"] = append(groups[s.kind+" ("+state+")"], s)
	}
	stats := map[string]any{}
	for k, g := range groups {
		ds := make([]float64, 0, len(g))
		bs := make([]float64, 0, len(g))
		errs := 0
		for _, s := range g {
			if s.status != 200 {
				errs++
				continue
			}
			ds = append(ds, float64(s.dur)/float64(time.Millisecond))
			bs = append(bs, float64(s.bytes))
		}
		sort.Float64s(ds)
		sort.Float64s(bs)
		pct := func(a []float64, p float64) float64 {
			if len(a) == 0 {
				return 0
			}
			return a[min(len(a)-1, int(p*float64(len(a))))]
		}
		stats[k] = map[string]any{"n": len(g), "errors": errs, "p50_ms": pct(ds, .5), "p95_ms": pct(ds, .95), "p99_ms": pct(ds, .99), "max_ms": pct(ds, 1),
			"bytes_p50": pct(bs, .5), "bytes_max": pct(bs, 1), "rps": float64(len(g)) / time.Since(start).Seconds()}
	}
	res["stats"] = stats
	res["small_bodies"] = small
	bt := make([]float64, 0, *members)
	for _, d := range builtAt {
		bt = append(bt, d.Seconds())
	}
	res["first_ready_seconds_per_member"] = bt
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
	if *out != "" {
		_ = os.WriteFile(*out, b, 0o644)
	}
}
