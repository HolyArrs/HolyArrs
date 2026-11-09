package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// The memory runs (3.7.10, decision 0007 "Switches" and "Memory limit
// setting"): the same fixture Library is loaded into a fresh app with
// every part on, and into one with only Movies and Shows on, each in a
// process of its own, and the memory each holds is measured: the Go heap,
// all memory the Go runtime has mapped (what GOMEMLIMIT sees), the
// process's resident memory (Linux), and the resident memory outside the
// Go runtime, which is mostly the SQLite driver's own heap and page caches
// (the driver is built without SQLite's memory counters, so it is
// measured from outside: resident memory minus what the Go runtime holds).
// No line for the difference between the two runs (no saving is claimed);
// each run must stay within decision 0002's limits (250 MB idle, 500 MB
// peak). The numbers go to the release report.
//
//	HOLYARRS_MEMORY=1 HOLYARRS_MEMORY_OUT=mem.txt go test -count=1 -run TestMemoryPartsRuns -v ./cmd/holyarrs
const (
	EnvMemory      = "HOLYARRS_MEMORY"
	EnvMemoryOut   = "HOLYARRS_MEMORY_OUT"
	envMemoryChild = "HOLYARRS_MEMORY_CHILD"
)

// Fixture size: about a real home Library.
const (
	fixtureMovies   = 2000
	fixtureShows    = 150
	fixtureSeasons  = 6
	fixtureEpisodes = 12
)

type memResult struct {
	Mode        string `json:"mode"`
	HeapLive    uint64 `json:"heap_live"`    // /gc/heap/live:bytes after GC
	HeapObjects uint64 `json:"heap_objects"` // /memory/classes/heap/objects:bytes
	GoTotal     uint64 `json:"go_total"`     // /memory/classes/total:bytes (what GOMEMLIMIT counts)
	GoReleased  uint64 `json:"go_released"`  // /memory/classes/heap/released:bytes (mapped, given back)
	RSS         uint64 `json:"rss"`          // VmRSS
	PeakRSS     uint64 `json:"peak_rss"`     // VmHWM
	// Outside is resident memory the Go runtime did not map (RSS minus
	// GoTotal): the SQLite driver's own allocations (page caches, its C
	// heap), which GOMEMLIMIT cannot see, plus the program's code.
	// OutsideBase is the same before the app started, so Outside minus
	// OutsideBase is what the app's SQLite use added.
	Outside       int64 `json:"outside"`
	OutsideBase   int64 `json:"outside_base"`
	Goroutines    int   `json:"goroutines"`
	Items         int   `json:"items"`
	FixtureMillis int64 `json:"fixture_ms"`
}

// memModes are measured in turn: all parts, Movies and Shows only, and all
// parts in low-memory mode.
var memModes = []string{"all", "movies-shows", "all-low"}

// TestMemoryPartsRuns runs the child three times per mode, alternating,
// and reports each run and the medians.
func TestMemoryPartsRuns(t *testing.T) {
	if os.Getenv(EnvMemory) == "" {
		t.Skip("set " + EnvMemory + "=1 to measure memory with all parts on and with Movies and Shows only")
	}
	var lines []string
	all := map[string][]memResult{}
	for round := range 3 {
		for _, mode := range memModes {
			cmd := testCommand(os.Args[0], "-test.run=^TestMemoryChild$", "-test.v", "-test.count=1", "-test.timeout=10m")
			cmd.Env = append(os.Environ(), envMemoryChild+"="+mode)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("round %d %s: %v\n%s", round, mode, err, out)
			}
			var r memResult
			sc := bufio.NewScanner(strings.NewReader(string(out)))
			for sc.Scan() {
				if _, js, ok := strings.Cut(sc.Text(), "MEMRESULT "); ok {
					if err := json.Unmarshal([]byte(js), &r); err != nil {
						t.Fatal(err)
					}
				}
			}
			if r.Mode == "" {
				t.Fatalf("no result:\n%s", out)
			}
			all[mode] = append(all[mode], r)
			lines = append(lines, row(fmt.Sprintf("%s, run %d", mode, round+1), r))
		}
	}
	for _, mode := range memModes {
		m := median(all[mode])
		lines = append(lines, row(mode+", median", m))
		// The pass lines of decision 0002: idle under 250 MB, peak under
		// 500 MB (resident memory, Linux only).
		if m.RSS > 250e6 || m.PeakRSS > 500e6 {
			t.Errorf("%s: idle %s, peak %s; the limits are 250 MB idle and 500 MB peak", mode, mib(m.RSS), mib(m.PeakRSS))
		}
	}
	head := fmt.Sprintf("%s %s/%s %s, GOMEMLIMIT=%q, fixture %d movies, %d shows x %d seasons x %d episodes",
		time.Now().UTC().Format(time.RFC3339), runtime.GOOS, runtime.GOARCH, runtime.Version(), os.Getenv("GOMEMLIMIT"),
		fixtureMovies, fixtureShows, fixtureSeasons, fixtureEpisodes)
	text := head + "\n| run | Go heap live | Go heap objects | Go runtime total | given back | resident (RSS) | peak RSS | outside Go | outside Go before start | goroutines |\n|---|---|---|---|---|---|---|---|---|---|\n" + strings.Join(lines, "\n") + "\n"
	t.Log("\n" + text)
	if out := os.Getenv(EnvMemoryOut); out != "" {
		if err := os.WriteFile(out, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func mib(b uint64) string { return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20)) }

func smib(b int64) string { return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20)) }

func row(name string, r memResult) string {
	rss, peak := "n/a", "n/a"
	if r.RSS > 0 {
		rss, peak = mib(r.RSS), mib(r.PeakRSS)
	}
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %d |", name, mib(r.HeapLive), mib(r.HeapObjects), mib(r.GoTotal),
		mib(r.GoReleased), rss, peak, smib(r.Outside), smib(r.OutsideBase), r.Goroutines)
}

func median(rs []memResult) memResult {
	pick := func(f func(memResult) uint64) uint64 {
		v := make([]uint64, len(rs))
		for i, r := range rs {
			v[i] = f(r)
		}
		slices.Sort(v)
		return v[len(v)/2]
	}
	return memResult{
		HeapLive:    pick(func(r memResult) uint64 { return r.HeapLive }),
		HeapObjects: pick(func(r memResult) uint64 { return r.HeapObjects }),
		GoTotal:     pick(func(r memResult) uint64 { return r.GoTotal }),
		GoReleased:  pick(func(r memResult) uint64 { return r.GoReleased }),
		RSS:         pick(func(r memResult) uint64 { return r.RSS }),
		PeakRSS:     pick(func(r memResult) uint64 { return r.PeakRSS }),
		Outside:     int64(pick(func(r memResult) uint64 { return uint64(max(r.Outside, 0)) })),
		OutsideBase: int64(pick(func(r memResult) uint64 { return uint64(max(r.OutsideBase, 0)) })),
		Goroutines:  int(pick(func(r memResult) uint64 { return uint64(r.Goroutines) })),
	}
}

// TestMemoryChild is one measured run; only TestMemoryPartsRuns starts it.
func TestMemoryChild(t *testing.T) {
	mode := os.Getenv(envMemoryChild)
	if mode == "" {
		t.Skip("started by TestMemoryPartsRuns")
	}
	if strings.HasSuffix(mode, "-low") {
		startup.SetLowMemory(true)
		t.Cleanup(func() { startup.SetLowMemory(false) })
	}
	base := measure(mode)
	a := startFaultApp(t, runOpts{})
	ctx := context.Background()
	if mode == "movies-shows" {
		for _, p := range []parts.Part{parts.Subtitles, parts.Wishlist, parts.Music} {
			if err := a.pc.Set(ctx, p, false, domain.System); err != nil {
				t.Fatalf("turn %s off: %v", p, err)
			}
		}
	}
	began := time.Now()
	items := loadFixture(t, a.pc.Store, a.dir)
	fixture := time.Since(began)
	// The same reads in both runs: the Library, the calendar, the health
	// states, a few title pages; then let the loops run a while.
	c := newClient()
	st, body, _ := a.call(c, "", "POST", "/api/v1/auth/login", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 200 {
		t.Fatalf("sign-in %d %s", st, body)
	}
	for range 3 {
		for _, p := range []string{"/api/v1/titles", "/api/v1/titles?kind=show", "/api/v1/calendar?start=2024-01-01&end=2024-03-01", "/api/v1/health/states", "/api/v1/parts", "/api/v1/titles/1", "/api/v1/titles/" + strconv.Itoa(fixtureMovies+1)} {
			if st, body, _ := a.call(c, "", "GET", p, ""); st != 200 {
				t.Fatalf("%s: %d %s", p, st, body[:min(len(body), 200)])
			}
		}
	}
	time.Sleep(15 * time.Second)
	runtime.GC()
	runtime.GC()
	r := measure(mode)
	r.Items, r.FixtureMillis, r.OutsideBase = items, fixture.Milliseconds(), base.Outside
	b, _ := json.Marshal(r)
	fmt.Println("MEMRESULT " + string(b))
}

func measure(mode string) memResult {
	names := []string{"/gc/heap/live:bytes", "/memory/classes/heap/objects:bytes", "/memory/classes/total:bytes", "/memory/classes/heap/released:bytes"}
	s := make([]metrics.Sample, len(names))
	for i, n := range names {
		s[i].Name = n
	}
	metrics.Read(s)
	r := memResult{Mode: mode, HeapLive: s[0].Value.Uint64(), HeapObjects: s[1].Value.Uint64(), GoTotal: s[2].Value.Uint64(),
		GoReleased: s[3].Value.Uint64(), Goroutines: runtime.NumGoroutine()}
	r.RSS, r.PeakRSS = procStatus("VmRSS:"), procStatus("VmHWM:")
	if r.RSS > 0 {
		r.Outside = int64(r.RSS) - int64(r.GoTotal-r.GoReleased)
	}
	return r
}

// procStatus reads one kB line of /proc/self/status (Linux), in bytes.
func procStatus(key string) uint64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, l := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(l, key); ok {
			n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
			return n * 1024
		}
	}
	return 0
}

// loadFixture adds the Library: movies with a file each, and shows with
// seasons of episodes, half of them with files. It returns the Items.
func loadFixture(t *testing.T, s *database.Store, dir string) int {
	t.Helper()
	ctx := context.Background()
	items := 0
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var rules int64
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		folder := func(kind, label string) (int64, error) {
			p := filepath.Join(dir, "media", kind)
			if err := os.MkdirAll(p, 0o755); err != nil {
				return 0, err
			}
			return database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: p, Kind: kind, Label: label, Health: "ok", SameDiskAs: []int64{}}, domain.System)
		}
		mf, err := folder("movie", "Movies")
		if err != nil {
			return err
		}
		sf, err := folder("show", "Shows")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		file := func(slot, ver int64, rel string) error {
			name := rel
			id, err := database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{slot}, VersionID: ver, RelativePath: rel, Size: 4 << 30, AddedAt: now,
				Languages: []string{"en"}, MediaInfo: map[string]any{}, ReleaseName: &name, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}, domain.System)
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", id, slot)
			return err
		}
		for i := range fixtureMovies {
			year := 1960 + i%60
			name := fmt.Sprintf("Fixture Movie %04d", i)
			overview := strings.Repeat("A plain overview of a movie for the memory run. ", 6)
			tid, err := database.Insert(tx, "titles", &mediafiles.Title{Kind: "movie", Name: name, SortName: name, Year: &year, Overview: &overview, Aliases: []string{},
				SourceIDs: map[string]string{"tmdb": strconv.Itoa(100000 + i)}, MetadataSource: "tmdb", KeepUpToDate: true, NewPartsPolicy: "all",
				ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", GateDriver: "none"}, domain.System)
			if err != nil {
				return err
			}
			folderName := fmt.Sprintf("%s (%d)", name, year)
			ver, err := database.Insert(tx, "versions", &mediafiles.Version{TitleID: tid, Label: "HD", PickRulesID: rules, LibraryFolderID: mf, FolderName: folderName, KeepUpToDate: true, IsMain: true}, domain.System)
			if err != nil {
				return err
			}
			item, err := database.Insert(tx, "items", &mediafiles.Item{TitleID: tid, Kind: "movie", KeepUpToDate: true, SourceIDs: map[string]string{}}, domain.System)
			if err != nil {
				return err
			}
			slot, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: item, VersionID: ver, State: "wanted"}, domain.System)
			if err != nil {
				return err
			}
			items++
			if i%5 != 0 { // four in five are in the Library
				if err := file(slot, ver, folderName+"/"+folderName+".mkv"); err != nil {
					return err
				}
			}
		}
		for i := range fixtureShows {
			year := 1990 + i%30
			name := fmt.Sprintf("Fixture Show %03d", i)
			tid, err := database.Insert(tx, "titles", &mediafiles.Title{Kind: "show", Name: name, SortName: name, Year: &year, Aliases: []string{},
				SourceIDs: map[string]string{"tmdb": strconv.Itoa(200000 + i)}, MetadataSource: "tmdb", KeepUpToDate: true, NewPartsPolicy: "all",
				ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", GateDriver: "none"}, domain.System)
			if err != nil {
				return err
			}
			folderName := fmt.Sprintf("%s (%d)", name, year)
			ver, err := database.Insert(tx, "versions", &mediafiles.Version{TitleID: tid, Label: "HD", PickRulesID: rules, LibraryFolderID: sf, FolderName: folderName, KeepUpToDate: true, IsMain: true}, domain.System)
			if err != nil {
				return err
			}
			for se := 1; se <= fixtureSeasons; se++ {
				for ep := 1; ep <= fixtureEpisodes; ep++ {
					se, ep := se, ep
					epName := fmt.Sprintf("Episode %d", ep)
					air := time.Date(2010+se, time.Month(1+ep%12), 1+ep, 0, 0, 0, 0, time.UTC)
					item, err := database.Insert(tx, "items", &mediafiles.Item{TitleID: tid, Kind: "episode", Numbers: mediafiles.Numbers{Season: &se, Episode: &ep},
						Name: &epName, AirDate: &air, KeepUpToDate: true, SourceIDs: map[string]string{}}, domain.System)
					if err != nil {
						return err
					}
					slot, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: item, VersionID: ver, State: "wanted"}, domain.System)
					if err != nil {
						return err
					}
					items++
					if ep%2 == 0 {
						if err := file(slot, ver, fmt.Sprintf("%s/Season %02d/%s - S%02dE%02d.mkv", folderName, se, name, se, ep)); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return items
}
