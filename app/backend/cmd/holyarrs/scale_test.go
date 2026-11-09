package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// The scale checks are intentionally opt-in: each one creates a large fresh
// database and starts the real application around it. They use only generated
// records and loopback fake feeds.
//
//	HOLYARRS_SCALE=all go test -count=1 -run '^TestScaleAcceptance$' -v ./cmd/holyarrs
const (
	envScale            = "HOLYARRS_SCALE"
	envScaleChild       = "HOLYARRS_SCALE_CHILD"
	envScaleIdle        = "HOLYARRS_SCALE_IDLE"
	envScaleSeedChild   = "HOLYARRS_SCALE_SEED_CHILD"
	envScaleDir         = "HOLYARRS_SCALE_DIR"
	envScaleFeed        = "HOLYARRS_SCALE_FEED"
	envScaleMedia       = "HOLYARRS_SCALE_MEDIA"
	envScaleBrowserAddr = "HOLYARRS_SCALE_BROWSER_ADDR"
	envScaleBrowserDir  = "HOLYARRS_SCALE_BROWSER_DIR"
)

type scaleSpec struct {
	Name     string
	Titles   int
	Items    int
	Feeds    int
	ItemKind string
}

var scaleSpecs = []scaleSpec{
	{Name: "movies", Titles: 5000, Items: 5000, Feeds: 20, ItemKind: "movie"},
	{Name: "shows", Titles: 5000, Items: 50000, ItemKind: "episode"},
	{Name: "music", Titles: 2000, Items: 200000, ItemKind: "track"},
	{Name: "combined", Titles: 10000, Items: 55000, Feeds: 20},
}

type scaleResult struct {
	Name                  string               `json:"name"`
	Titles                int                  `json:"titles"`
	Items                 int                  `json:"items"`
	Feeds                 int                  `json:"feeds"`
	FeedRequests          int                  `json:"feed_requests"`
	InitialFeedRequests   int                  `json:"initial_feed_requests"`
	ScheduledFeedRequests int                  `json:"scheduled_feed_requests"`
	SeedMillis            int64                `json:"seed_ms"`
	FixturePeakRSS        uint64               `json:"fixture_peak_rss"`
	RescanMillis          int64                `json:"rescan_ms"`
	RescanFolders         int                  `json:"rescan_folders"`
	RescanFiles           int                  `json:"rescan_files"`
	RescanFound           int                  `json:"rescan_found,omitempty"`
	RescanUnreachable     int                  `json:"rescan_unreachable"`
	LiveClients           int                  `json:"live_clients,omitempty"`
	LiveRescanEvents      int                  `json:"live_rescan_events,omitempty"`
	BrowserTabs           int                  `json:"browser_tabs,omitempty"`
	BrowserLiveStreams    int                  `json:"browser_live_streams,omitempty"`
	FullTextStatus        string               `json:"full_text_status"`
	FullTextRebuildMillis int64                `json:"full_text_rebuild_ms"`
	HealthMillis          int64                `json:"health_ms"`
	WorkingWrites         int64                `json:"working_writes"`
	WorkingWriteBytes     uint64               `json:"working_write_bytes"`
	IdleWrites            int64                `json:"idle_writes"`
	IdleWriteBytes        uint64               `json:"idle_write_bytes"`
	WriteWaitP95MS        float64              `json:"write_wait_p95_ms"`
	WriteWaitSamples      int                  `json:"write_wait_samples"`
	IdleSeconds           int64                `json:"idle_seconds"`
	IdleRSSMedian         uint64               `json:"idle_rss_median"`
	IdleRSSMax            uint64               `json:"idle_rss_max"`
	RSS                   uint64               `json:"rss"`
	PeakRSS               uint64               `json:"peak_rss"`
	RescanPeakRSS         uint64               `json:"rescan_peak_rss"`
	DatabaseBytes         int64                `json:"database_bytes"`
	IdleJobsUpdated       []scaleJobDiagnostic `json:"idle_jobs_updated"`
}

type scaleJobDiagnostic struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	State   string `json:"state"`
	Attempt int    `json:"attempt"`
}

type scaleSeedResult struct {
	Millis  int64  `json:"seed_ms"`
	PeakRSS uint64 `json:"fixture_peak_rss"`
}

type scaleBrowserProof struct {
	Status           string `json:"status"`
	Tabs             int    `json:"tabs"`
	LiveStreams      int    `json:"live_streams"`
	LiveRescanEvents int    `json:"live_rescan_events"`
}

func TestScaleAcceptance(t *testing.T) {
	wanted := os.Getenv(envScale)
	if wanted == "" {
		t.Skip("set " + envScale + "=all (or movies, shows, music, combined) to run the large application checks")
	}
	selected := map[string]bool{}
	for _, name := range strings.Split(wanted, ",") {
		selected[strings.TrimSpace(name)] = true
	}
	for _, spec := range scaleSpecs {
		if !selected["all"] && !selected[spec.Name] {
			continue
		}
		t.Run(spec.Name, func(t *testing.T) {
			t.Parallel()
			cmd := testCommand(os.Args[0], "-test.run=^TestScaleAcceptanceChild$", "-test.v", "-test.count=1", "-test.timeout="+scaleChildTimeout(spec))
			cmd.Env = append(os.Environ(), envScaleChild+"="+spec.Name)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s scale child: %v\n%s", spec.Name, err, out)
			}
			var result scaleResult
			scanner := bufio.NewScanner(strings.NewReader(string(out)))
			for scanner.Scan() {
				if _, body, ok := strings.Cut(scanner.Text(), "SCALERESULT "); ok {
					if err := json.Unmarshal([]byte(body), &result); err != nil {
						t.Fatal(err)
					}
				}
			}
			if result.Name == "" {
				t.Fatalf("%s scale child returned no result:\n%s", spec.Name, out)
			}
			b, _ := json.Marshal(result)
			t.Log("SCALERESULT " + string(b))
		})
	}
}

func TestScaleAcceptanceChild(t *testing.T) {
	name := os.Getenv(envScaleChild)
	if name == "" {
		t.Skip("started by TestScaleAcceptance")
	}
	var spec scaleSpec
	for _, candidate := range scaleSpecs {
		if candidate.Name == name {
			spec = candidate
			break
		}
	}
	if spec.Name == "" {
		t.Fatalf("unknown scale check %q", name)
	}
	if runtime.GOOS != "linux" {
		t.Fatal("scale acceptance needs Linux /proc metrics")
	}
	idleFor := 30 * time.Minute
	if raw := os.Getenv(envScaleIdle); raw != "" {
		var err error
		idleFor, err = time.ParseDuration(raw)
		if err != nil || idleFor <= 0 {
			t.Fatalf("%s must be a positive duration: %q", envScaleIdle, raw)
		}
	}

	var feedRequests int
	var feedMu sync.Mutex
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		feedMu.Lock()
		feedRequests++
		feedMu.Unlock()
		if spec.Name == "combined" {
			_, _ = w.Write([]byte(combinedPinnedFeed))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><item><title>Fixture.2026.1080p</title><link>http://fixture.invalid/item.torrent</link><guid>fixture</guid><pubDate>Tue, 13 Oct 2026 12:00:00 +0000</pubDate></item></channel></rss>`))
	}))
	defer feed.Close()

	dir := t.TempDir()
	mediaRoot := filepath.Join(dir, "fixture-media")
	seedTimeout := "20m"
	if spec.Name == "combined" {
		seedTimeout = "30m"
	}
	seedCmd := testCommand(os.Args[0], "-test.run=^TestScaleSeedChild$", "-test.v", "-test.count=1", "-test.timeout="+seedTimeout)
	seedCmd.Env = append(os.Environ(), envScaleSeedChild+"="+spec.Name, envScaleDir+"="+dir, envScaleFeed+"="+feed.URL, envScaleMedia+"="+mediaRoot)
	seedOut, err := seedCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("seed scale fixture: %v\n%s", err, seedOut)
	}
	var seedResult scaleSeedResult
	seedScan := bufio.NewScanner(strings.NewReader(string(seedOut)))
	for seedScan.Scan() {
		if _, body, ok := strings.Cut(seedScan.Text(), "SCALESEED "); ok {
			if err := json.Unmarshal([]byte(body), &seedResult); err != nil {
				t.Fatal(err)
			}
		}
	}
	if seedResult.PeakRSS == 0 {
		t.Fatalf("seed child returned no metrics:\n%s", seedOut)
	}
	workingIO := procWriteBytes(t)

	var control *parts.Control
	reg, addr := startScaleApp(t, config{ConfigDir: dir}, runOpts{parts: func(c *parts.Control) { control = c }})
	a := &faultApp{t: t, base: "http://" + addr, dir: dir, pc: control}
	client := newClient()
	status, body, _ := a.call(client, "", "POST", "/api/v1/auth/setup", `{"name":"scale","password":"a long generated passphrase"}`)
	if status != http.StatusCreated {
		t.Fatalf("setup %d %s", status, body)
	}
	var owner struct {
		CSRF string `json:"csrf"`
	}
	if err := json.Unmarshal([]byte(body), &owner); err != nil || owner.CSRF == "" {
		t.Fatalf("setup response has no CSRF token: %v %s", err, body)
	}
	fullTextStatus := "not measured outside the combined check"
	var fullTextRebuildMillis int64
	if spec.Name == "combined" {
		probe := scaleFullTextProbe(spec)
		fullTextStarted := time.Now()
		if err := mediafiles.RebuildTitleSearch(context.Background(), control.Store); err != nil {
			t.Fatalf("rebuild title search: %v", err)
		}
		fullTextRebuildMillis = time.Since(fullTextStarted).Milliseconds()
		if matches, err := mediafiles.SearchTitles(context.Background(), control.Store, probe); err != nil || len(matches) != 1 || matches[0].Name != probe {
			t.Fatalf("rebuilt title search returned %d matches (%v): %v", len(matches), matches, err)
		}
		fullTextStatus = "available: rebuilt and queried"
	}

	storedSources, err := database.Select[domain.Source](context.Background(), control.Store.Read(), "sources", "format = 'rss' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	if len(storedSources) != spec.Feeds {
		t.Fatalf("stored fixture Sources %d, want %d", len(storedSources), spec.Feeds)
	}
	feedClients := make([]*sources.Client, len(storedSources))
	for i, src := range storedSources {
		cfg, err := sources.ConfigFor(context.Background(), src, nil)
		if err != nil {
			t.Fatal(err)
		}
		feedClients[i] = sources.New(cfg)
	}
	var feedWG sync.WaitGroup
	feedErrs := make(chan error, len(feedClients))
	for _, sourceClient := range feedClients {
		feedWG.Add(1)
		go func() {
			defer feedWG.Done()
			items, err := sourceClient.Feed(context.Background(), nil)
			want := 1
			if spec.Name == "combined" {
				want = 2
			}
			if err == nil && len(items) != want {
				err = fmt.Errorf("fake feed returned %d items, want %d", len(items), want)
			}
			feedErrs <- err
		}()
	}
	feedWG.Wait()
	close(feedErrs)
	for err := range feedErrs {
		if err != nil {
			t.Fatal(err)
		}
	}

	lockErrs := make(chan error, 100)
	var lockWG sync.WaitGroup
	for i := range 100 {
		lockWG.Add(1)
		go func() {
			defer lockWG.Done()
			lockErrs <- control.Store.Write(context.Background(), func(tx *sql.Tx) error {
				_, err := tx.Exec("UPDATE titles SET updated_at = ? WHERE id = ?", database.FormatTime(time.Now().UTC()), 1+i%spec.Titles)
				return err
			})
		}()
	}
	lockWG.Wait()
	close(lockErrs)
	for err := range lockErrs {
		if err != nil {
			t.Fatal(err)
		}
	}
	writeP95, writeSamples := control.Store.WriteWaitP95()
	var knownFiles int
	if err := control.Store.Read().QueryRow("SELECT count(*) FROM files WHERE state IN ('present', 'missing')").Scan(&knownFiles); err != nil {
		t.Fatal(err)
	}
	var liveStreams *scaleLiveStreams
	var browserProof scaleBrowserProof
	if spec.Name == "combined" {
		// The measured rescan starts after startup work has honestly settled,
		// so its two newly-created unknown files and live event are its own.
		waitForScaleQuiet(t, control.Store, reg, 10*time.Minute)
		if browserScaleEnabled(t) {
			writeScaleBrowserProof(t, "app-ready.json", scaleBrowserProof{Status: "ready"})
			browserProof = waitScaleBrowserProof(t, "browser-ready.json", 2*time.Minute)
			if browserProof.Status != "ready" || browserProof.Tabs != 10 || browserProof.LiveStreams != 10 {
				t.Fatalf("rendered browser readiness %+v, want ready with 10 tabs and 10 live streams", browserProof)
			}
		} else {
			liveStreams = openScaleLiveStreams(t, a, 10)
			defer liveStreams.Close()
		}
		if err := addCombinedUnknownFiles(mediaRoot); err != nil {
			t.Fatal(err)
		}
	}
	rescanStarted := time.Now()
	stopRescanRSS := make(chan struct{})
	rescanRSS := make(chan uint64, 1)
	go func() {
		peak := procStatus("VmRSS:")
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				peak = max(peak, procStatus("VmRSS:"))
			case <-stopRescanRSS:
				rescanRSS <- max(peak, procStatus("VmRSS:"))
				return
			}
		}
	}()
	var rescan library.RescanResult
	if spec.Name == "combined" {
		rescan, err = runScaleRescanAPI(t, a, client, owner.CSRF, 10*time.Minute)
	} else {
		rescan, err = (&library.Service{Store: control.Store}).Rescan(context.Background(), domain.System)
	}
	close(stopRescanRSS)
	rescanPeakRSS := <-rescanRSS
	if err != nil {
		t.Fatal(err)
	}
	rescanMillis := time.Since(rescanStarted).Milliseconds()
	if spec.Name == "combined" && browserScaleEnabled(t) {
		browserProof = waitScaleBrowserRescanProof(t, 2*time.Minute)
	}

	healthStarted := time.Now()
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status %d", resp.StatusCode)
	}
	healthMillis := time.Since(healthStarted).Milliseconds()

	titles, items := scaleCounts(t, control.Store, spec)
	if titles != spec.Titles || items != spec.Items {
		t.Fatalf("loaded %d titles/%d items, want %d/%d", titles, items, spec.Titles, spec.Items)
	}
	feedMu.Lock()
	initialRequests := feedRequests
	feedMu.Unlock()
	if initialRequests < spec.Feeds {
		t.Fatalf("fake feed requests %d, want at least the %d explicit requests", initialRequests, spec.Feeds)
	}

	workingWrites := control.Store.WriteCount()
	workingWriteBytes := procWriteBytes(t) - workingIO
	if workingWrites == 0 || workingWriteBytes == 0 {
		t.Fatalf("working measurements are empty: %d commits, %d write bytes", workingWrites, workingWriteBytes)
	}

	// Startup may finish queued housekeeping such as the first backup. It is
	// not idle until no queued or running Job remains and both application
	// commits and kernel write bytes have been unchanged for ten seconds.
	waitForScaleQuiet(t, control.Store, reg, 10*time.Minute)
	idleStartedAt := time.Now().UTC()
	idleWritesAt := control.Store.WriteCount()
	idleIOAt := procWriteBytes(t)
	feedMu.Lock()
	idleFeedAt := feedRequests
	feedMu.Unlock()
	var rssSamples []uint64
	idleDeadline := time.Now().Add(idleFor)
	for {
		rssSamples = append(rssSamples, procStatus("VmRSS:"))
		left := time.Until(idleDeadline)
		if left <= 0 {
			break
		}
		time.Sleep(min(10*time.Second, left))
	}
	idleWrites := control.Store.WriteCount() - idleWritesAt
	idleWriteBytes := procWriteBytes(t) - idleIOAt
	idleJobsUpdated, err := scaleJobsUpdatedSince(control.Store, idleStartedAt)
	if err != nil {
		t.Fatal(err)
	}
	feedMu.Lock()
	requests := feedRequests
	feedMu.Unlock()
	if spec.Name == "combined" && browserScaleEnabled(t) {
		writeScaleBrowserProof(t, "app-done.json", scaleBrowserProof{Status: "done"})
		browserProof = waitScaleBrowserProof(t, "browser-result.json", 2*time.Minute)
		if browserProof.Status != "passed" || browserProof.Tabs != 10 || browserProof.LiveStreams != 10 || browserProof.LiveRescanEvents != 10 {
			t.Fatalf("rendered browser result %+v, want passed with 10 tabs, streams, and rescan events", browserProof)
		}
	}

	rssSamples = append(rssSamples, procStatus("VmRSS:"))
	sort.Slice(rssSamples, func(i, j int) bool { return rssSamples[i] < rssSamples[j] })
	idleMedian := rssSamples[len(rssSamples)/2]
	idleMax := rssSamples[len(rssSamples)-1]
	result := scaleResult{
		Name: spec.Name, Titles: titles, Items: items, Feeds: spec.Feeds, FeedRequests: requests, InitialFeedRequests: initialRequests, ScheduledFeedRequests: requests - idleFeedAt,
		SeedMillis: seedResult.Millis, FixturePeakRSS: seedResult.PeakRSS, RescanMillis: rescanMillis, RescanFolders: rescan.Folders, RescanFiles: knownFiles,
		FullTextStatus:        fullTextStatus,
		FullTextRebuildMillis: fullTextRebuildMillis,
		HealthMillis:          healthMillis, WorkingWrites: workingWrites, WorkingWriteBytes: workingWriteBytes,
		IdleWrites: idleWrites, IdleWriteBytes: idleWriteBytes, WriteWaitP95MS: float64(writeP95.Microseconds()) / 1000,
		WriteWaitSamples: writeSamples, IdleSeconds: int64(idleFor / time.Second), IdleRSSMedian: idleMedian, IdleRSSMax: idleMax,
		RSS: procStatus("VmRSS:"), PeakRSS: procStatus("VmHWM:"), RescanPeakRSS: rescanPeakRSS, DatabaseBytes: databaseBytes(t, dir),
		IdleJobsUpdated: idleJobsUpdated,
	}
	if spec.Name == "combined" {
		result.RescanFound = rescan.Found
		result.RescanUnreachable = rescan.Unreachable
		if browserScaleEnabled(t) {
			result.LiveClients = browserProof.LiveStreams
			result.LiveRescanEvents = browserProof.LiveRescanEvents
			result.BrowserTabs = browserProof.Tabs
			result.BrowserLiveStreams = browserProof.LiveStreams
		} else {
			result.LiveClients = liveStreams.Count()
			result.LiveRescanEvents = liveStreams.RescanEvents()
		}
	}
	b, _ := json.Marshal(result)
	fmt.Println("SCALERESULT " + string(b))

	if result.RSS == 0 || result.PeakRSS == 0 || result.RescanPeakRSS == 0 {
		t.Error("Linux resident-memory metrics are unavailable")
	}
	if spec.Name == "combined" && (result.FullTextStatus != "available: rebuilt and queried" || result.FullTextRebuildMillis <= 0) {
		t.Errorf("full-text rebuild status=%q duration=%dms", result.FullTextStatus, result.FullTextRebuildMillis)
	}
	if result.IdleRSSMax >= 250e6 {
		t.Errorf("maximum idle RSS %s, pass line is under 250 MB", mib(result.IdleRSSMax))
	}
	if result.RescanPeakRSS >= 500e6 {
		t.Errorf("rescan peak RSS %s, pass line is under 500 MB", mib(result.RescanPeakRSS))
	}
	if result.PeakRSS >= 500e6 {
		t.Errorf("whole-application peak RSS %s, pass line is under 500 MB", mib(result.PeakRSS))
	}
	if spec.Feeds > 0 && idleFor >= 15*time.Minute && result.ScheduledFeedRequests < spec.Feeds {
		t.Errorf("scheduled feeds made %d requests during %v, want at least one %d-Source pass", result.ScheduledFeedRequests, idleFor, spec.Feeds)
	}
	if result.IdleWrites != 0 || result.IdleWriteBytes != 0 {
		t.Errorf("idle application wrote %d transactions and %d bytes", result.IdleWrites, result.IdleWriteBytes)
	}
	if writeSamples < 100 || writeP95 >= 100*time.Millisecond {
		t.Errorf("writer wait p95 %v over %d samples, pass line is under 100ms", writeP95, writeSamples)
	}
	if spec.Name == "combined" {
		if result.RescanFolders != 2 || result.RescanFiles != combinedKnownFiles(combinedScaleShape) || result.RescanFound != 2 || result.RescanUnreachable != 0 {
			t.Errorf("combined rescan folders=%d known=%d found=%d unreachable=%d", result.RescanFolders, result.RescanFiles, result.RescanFound, result.RescanUnreachable)
		}
		if result.LiveClients != 10 || result.LiveRescanEvents != 10 {
			t.Errorf("combined live streams clients=%d rescan_events=%d, want 10/10", result.LiveClients, result.LiveRescanEvents)
		}
	}
}

func startScaleApp(t *testing.T, c config, o runOpts) (*jobs.Registry, string) {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv(envScaleBrowserAddr))
	if addr == "" {
		return startApp(t, c, o)
	}
	if c.ConfigDir == "" {
		c.ConfigDir = t.TempDir()
	}
	c.Addr = addr
	type up struct {
		reg  *jobs.Registry
		addr string
	}
	ready := make(chan up, 1)
	o.ready = func(reg *jobs.Registry, actual string) { ready <- up{reg: reg, addr: actual} }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, c, o) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("the app did not stop")
		}
	})
	select {
	case started := <-ready:
		return started.reg, started.addr
	case err := <-done:
		t.Fatalf("serve ended: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("the app did not start")
	}
	return nil, ""
}

func scaleFullTextProbe(spec scaleSpec) string {
	switch spec.Name {
	case "combined":
		return fmt.Sprintf("Fixture Movie %04d", combinedScaleShape.Movies-1)
	case "movies":
		return fmt.Sprintf("Fixture Movie %04d", spec.Titles-1)
	case "shows":
		return fmt.Sprintf("Fixture Show %04d", spec.Titles-1)
	case "music":
		return fmt.Sprintf("Fixture Artist %04d", spec.Titles-1)
	default:
		return ""
	}
}

func browserScaleEnabled(t *testing.T) bool {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv(envScaleBrowserDir))
	addr := strings.TrimSpace(os.Getenv(envScaleBrowserAddr))
	if (dir == "") != (addr == "") {
		t.Fatalf("%s and %s must be set together", envScaleBrowserDir, envScaleBrowserAddr)
	}
	return dir != ""
}

func writeScaleBrowserProof(t *testing.T, name string, proof scaleBrowserProof) {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv(envScaleBrowserDir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, name+".tmp")
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func waitScaleBrowserProof(t *testing.T, name string, limit time.Duration) scaleBrowserProof {
	t.Helper()
	path := filepath.Join(strings.TrimSpace(os.Getenv(envScaleBrowserDir)), name)
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var proof scaleBrowserProof
			if err := json.Unmarshal(raw, &proof); err != nil {
				t.Fatalf("read rendered browser proof %s: %v", name, err)
			}
			return proof
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read rendered browser proof %s: %v", name, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("rendered browser proof %s did not arrive within %v", name, limit)
	return scaleBrowserProof{}
}

func waitScaleBrowserRescanProof(t *testing.T, limit time.Duration) scaleBrowserProof {
	t.Helper()
	proof := waitScaleBrowserProof(t, "rescan-events-ready.json", limit)
	if !validScaleBrowserRescanProof(proof) {
		t.Fatalf("rendered browser rescan proof %+v, want ready with 10 tabs, streams, and rescan events", proof)
	}
	return proof
}

func validScaleBrowserRescanProof(proof scaleBrowserProof) bool {
	return proof.Status == "ready" && proof.Tabs == 10 && proof.LiveStreams == 10 && proof.LiveRescanEvents == 10
}

func scaleChildTimeout(spec scaleSpec) string {
	if spec.Name == "combined" {
		return "70m"
	}
	return "45m"
}

func TestScaleSeedChild(t *testing.T) {
	name := os.Getenv(envScaleSeedChild)
	if name == "" {
		t.Skip("started by TestScaleAcceptanceChild")
	}
	var spec scaleSpec
	for _, candidate := range scaleSpecs {
		if candidate.Name == name {
			spec = candidate
			break
		}
	}
	if spec.Name == "" {
		t.Fatalf("unknown scale seed %q", name)
	}
	dir, feedURL, mediaRoot := os.Getenv(envScaleDir), os.Getenv(envScaleFeed), os.Getenv(envScaleMedia)
	if dir == "" || feedURL == "" || mediaRoot == "" {
		t.Fatal("scale seed paths are missing")
	}
	started := time.Now()
	s, err := database.Open(dir, database.Options{WriteCap: -1})
	if err != nil {
		t.Fatal(err)
	}
	if err := seedScale(t, s, spec, feedURL, mediaRoot); err != nil {
		_ = s.Close()
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	result := scaleSeedResult{Millis: time.Since(started).Milliseconds(), PeakRSS: procStatus("VmHWM:")}
	b, _ := json.Marshal(result)
	fmt.Println("SCALESEED " + string(b))
}

func seedScale(t *testing.T, s *database.Store, spec scaleSpec, feedURL, mediaRoot string) error {
	t.Helper()
	if spec.Name == "combined" {
		return seedCombinedScale(t, s, combinedScaleShape, feedURL, mediaRoot)
	}
	return s.Write(context.Background(), func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now().UTC())
		actor := `{"kind":"system"}`
		var rules int64
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rules); err != nil {
			return err
		}
		folderKind := map[string]string{"movies": "movie", "shows": "show", "music": "music"}[spec.Name]
		folderPath := filepath.Join(mediaRoot, folderKind)
		folderResult, err := tx.Exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, ?, ?, ?, ?, ?, ?, 'ok', '[]')`, now, actor, now, actor, folderPath, folderKind, strings.Title(folderKind))
		if err != nil {
			return err
		}
		folderID, _ := folderResult.LastInsertId()

		titleStmt, err := tx.Prepare(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, aliases, source_ids, metadata_source, keep_up_to_date, new_parts_policy, release_gate, music_types, why_added, artwork, status, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '[]', ?, ?, 1, 'all', 'released', ?, 'by_hand', '[]', ?, 'active')`)
		if err != nil {
			return err
		}
		defer titleStmt.Close()
		versionStmt, err := tx.Prepare(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1)`)
		if err != nil {
			return err
		}
		defer versionStmt.Close()
		partStmt, err := tx.Prepare(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, number, name, release_type, keep_up_to_date, source_ids, pressings) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, '[]')`)
		if err != nil {
			return err
		}
		defer partStmt.Close()
		itemStmt, err := tx.Prepare(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind, numbers, name, keep_up_to_date, recording_id, source_ids, length_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer itemStmt.Close()
		slotStmt, err := tx.Prepare(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'wanted')`)
		if err != nil {
			return err
		}
		defer slotStmt.Close()

		addTitle := func(kind, name, sourceIDs, metadata, musicTypes, status, label string, year int) (int64, int64, error) {
			row, err := titleStmt.Exec(now, actor, now, actor, kind, name, name, year, sourceIDs, metadata, musicTypes, status)
			if err != nil {
				return 0, 0, err
			}
			titleID, _ := row.LastInsertId()
			version, err := versionStmt.Exec(now, actor, now, actor, titleID, label, rules, folderID, name)
			if err != nil {
				return 0, 0, err
			}
			versionID, _ := version.LastInsertId()
			if err := os.MkdirAll(filepath.Join(folderPath, name), 0o755); err != nil {
				return 0, 0, err
			}
			return titleID, versionID, nil
		}
		addItem := func(titleID, versionID int64, partID any, kind, numbers, name string, recording any, sourceIDs string, length any) error {
			row, err := itemStmt.Exec(now, actor, now, actor, titleID, partID, kind, numbers, name, recording, sourceIDs, length)
			if err != nil {
				return err
			}
			itemID, _ := row.LastInsertId()
			_, err = slotStmt.Exec(now, actor, now, actor, itemID, versionID)
			return err
		}

		switch spec.Name {
		case "movies":
			for i := range spec.Titles {
				name := fmt.Sprintf("Fixture Movie %04d", i)
				titleID, versionID, err := addTitle("movie", name, fmt.Sprintf(`{"tmdb":"%d"}`, 100000+i), "tmdb", "[]", "released", "HD", 1960+i%60)
				if err != nil {
					return err
				}
				if err := addItem(titleID, versionID, nil, "movie", "{}", name, nil, "{}", nil); err != nil {
					return err
				}
			}
			for i := range spec.Feeds {
				_, err := tx.Exec(`INSERT INTO sources (created_at, created_by, updated_at, updated_by, name, protocol, format, url, use_for, kinds, categories, limits, priority, state, extra, allow_local_network) VALUES (?, ?, ?, ?, ?, 'torrent', 'rss', ?, '{"feed":true}', '["movie"]', '{}', '{}', 25, 'enabled', '{}', 1)`, now, actor, now, actor, fmt.Sprintf("Fixture feed %02d", i), fmt.Sprintf("%s/feed/%d", feedURL, i))
				if err != nil {
					return err
				}
			}
		case "shows":
			for i := range spec.Titles {
				name := fmt.Sprintf("Fixture Show %04d", i)
				titleID, versionID, err := addTitle("show", name, fmt.Sprintf(`{"tmdb":"%d"}`, 200000+i), "tmdb", "[]", "running", "HD", 1990+i%30)
				if err != nil {
					return err
				}
				part, err := partStmt.Exec(now, actor, now, actor, titleID, "season", 1, "Season 1", nil, `{}`, nil)
				if err != nil {
					return err
				}
				partID, _ := part.LastInsertId()
				for episode := 1; episode <= 10; episode++ {
					epName := fmt.Sprintf("Episode %d", episode)
					if err := addItem(titleID, versionID, partID, "episode", fmt.Sprintf(`{"season":1,"episode":%d}`, episode), epName, nil, fmt.Sprintf(`{"tmdb":"%d"}`, 300000+i*10+episode), nil); err != nil {
						return err
					}
				}
			}
		case "music":
			for i := range spec.Titles {
				name := fmt.Sprintf("Fixture Artist %04d", i)
				titleID, versionID, err := addTitle("artist", name, fmt.Sprintf(`{"musicbrainz":"artist-%d"}`, i), "musicbrainz", `["album"]`, "ended", "Lossless", 1980+i%40)
				if err != nil {
					return err
				}
				for album := 1; album <= 10; album++ {
					albumName := fmt.Sprintf("Album %02d", album)
					part, err := partStmt.Exec(now, actor, now, actor, titleID, "album", nil, albumName, "album", fmt.Sprintf(`{"musicbrainz":"album-%d-%d"}`, i, album), nil)
					if err != nil {
						return err
					}
					partID, _ := part.LastInsertId()
					for track := 1; track <= 10; track++ {
						trackName := fmt.Sprintf("Track %02d", track)
						recording := fmt.Sprintf("recording-%d-%d-%d", i, album, track)
						if err := addItem(titleID, versionID, partID, "track", fmt.Sprintf(`{"disc":1,"track":%d}`, track), trackName, recording, "{}", 180000); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	})
}

func scaleCounts(t *testing.T, s *database.Store, spec scaleSpec) (titles, items int) {
	t.Helper()
	if spec.Name == "combined" {
		if err := s.Read().QueryRow("SELECT count(*) FROM titles WHERE kind IN ('movie', 'show')").Scan(&titles); err != nil {
			t.Fatal(err)
		}
		if err := s.Read().QueryRow("SELECT count(*) FROM items WHERE kind IN ('movie', 'episode')").Scan(&items); err != nil {
			t.Fatal(err)
		}
		return titles, items
	}
	kind := map[string]string{"movies": "movie", "shows": "show", "music": "artist"}[spec.Name]
	if err := s.Read().QueryRow("SELECT count(*) FROM titles WHERE kind = ?", kind).Scan(&titles); err != nil {
		t.Fatal(err)
	}
	if err := s.Read().QueryRow("SELECT count(*) FROM items WHERE kind = ?", spec.ItemKind).Scan(&items); err != nil {
		t.Fatal(err)
	}
	return titles, items
}

func procWriteBytes(t *testing.T) uint64 {
	t.Helper()
	b, err := os.ReadFile("/proc/self/io")
	if err != nil {
		t.Fatalf("read /proc/self/io: %v", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if value, ok := strings.CutPrefix(line, "write_bytes:"); ok {
			n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatal("/proc/self/io has no write_bytes")
	return 0
}

func databaseBytes(t *testing.T, dir string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".db") && !strings.Contains(filepath.Base(path), ".db-")) {
			return nil
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func waitForScaleQuiet(t *testing.T, s *database.Store, reg *jobs.Registry, limit time.Duration) {
	t.Helper()
	deadline := time.Now().Add(limit)
	writes, bytes := s.WriteCount(), procWriteBytes(t)
	quietSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		nextWrites, nextBytes := s.WriteCount(), procWriteBytes(t)
		live, err := scaleLiveJobs(s)
		if err != nil {
			t.Fatal(err)
		}
		if nextWrites != writes || nextBytes != bytes || len(live) > 0 || !scaleSchedulerPassed(reg) {
			writes, bytes = nextWrites, nextBytes
			quietSince = time.Now()
			continue
		}
		if time.Since(quietSince) >= 10*time.Second {
			return
		}
	}
	live, err := scaleLiveJobs(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("application did not become write-idle within %v; schedules passed: %t; live jobs: %+v", limit, scaleSchedulerPassed(reg), live)
}

func scaleSchedulerPassed(reg *jobs.Registry) bool {
	if reg == nil {
		return false
	}
	status, ok := reg.Get(jobs.StageKey("schedules"))
	return ok && status.LastPass != nil
}

func scaleLiveJobs(s *database.Store) ([]scaleJobDiagnostic, error) {
	return scaleJobDiagnostics(s, "state IN ('queued', 'running')", nil)
}

func scaleJobsUpdatedSince(s *database.Store, since time.Time) ([]scaleJobDiagnostic, error) {
	return scaleJobDiagnostics(s, "updated_at >= ?", []any{database.FormatTime(since)})
}

func scaleJobDiagnostics(s *database.Store, where string, args []any) ([]scaleJobDiagnostic, error) {
	rows, err := s.Read().Query("SELECT id, kind, state, attempt FROM jobs WHERE "+where+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]scaleJobDiagnostic, 0)
	for rows.Next() {
		var job scaleJobDiagnostic
		if err := rows.Scan(&job.ID, &job.Kind, &job.State, &job.Attempt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
