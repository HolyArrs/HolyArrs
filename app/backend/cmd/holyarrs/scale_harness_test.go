package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

func TestScaleSeededSourceUsesItsStoredLocalNetworkPermission(t *testing.T) {
	var requests int
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><item><title>Fixture.2026.1080p</title><link>http://fixture.invalid/item.torrent</link><guid>fixture</guid></item></channel></rss>`))
	}))
	defer feed.Close()

	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	spec := scaleSpec{Name: "movies", Titles: 1, Items: 1, Feeds: 1, ItemKind: "movie"}
	if err := seedScale(t, s, spec, feed.URL, filepath.Join(t.TempDir(), "media")); err != nil {
		t.Fatal(err)
	}
	stored, err := database.Select[domain.Source](context.Background(), s.Read(), "sources", "1 ORDER BY id")
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored Sources %d: %v", len(stored), err)
	}
	cfg, err := sources.ConfigFor(context.Background(), stored[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := sources.New(cfg).Feed(context.Background(), nil)
	if err != nil || len(items) != 1 || requests != 1 {
		t.Fatalf("stored Source feed: %d items, %d requests, error %v", len(items), requests, err)
	}
}

func TestScaleLiveJobsBlocksOnlyQueuedAndRunning(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		for i, state := range []string{"queued", "running", "done", "failed", "cancelled"} {
			at := database.FormatTime(now.Add(time.Duration(i+1) * time.Hour))
			_, err := tx.Exec(`INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, state, attempt, run_after, asked_by) VALUES (?, '{"kind":"system"}', ?, '{"kind":"system"}', ?, ?, ?, ?, '{"kind":"system"}')`,
				database.FormatTime(now), database.FormatTime(now), "fixture-"+state, state, i+1, at)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	live, err := scaleLiveJobs(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 2 || live[0].Kind != "fixture-queued" || live[0].State != "queued" || live[1].Kind != "fixture-running" || live[1].State != "running" {
		t.Fatalf("live jobs %+v, want queued future and running only", live)
	}
}

func TestScaleQuietWaitsForFirstSchedulePass(t *testing.T) {
	reg := jobs.NewRegistry()
	reg.Set(jobs.StageKey("schedules"), "stage", "Schedules", jobs.Running, "")
	if scaleSchedulerPassed(reg) {
		t.Fatal("scheduler without a completed pass was treated as quiet")
	}
	now := time.Now().UTC()
	reg.Update(jobs.StageKey("schedules"), func(status *jobs.Status) { status.LastPass = &now })
	if !scaleSchedulerPassed(reg) {
		t.Fatal("completed first schedule pass did not unblock quiet detection")
	}
}

func TestScaleCombinedChildTimeoutIncludesQEMUHeadroom(t *testing.T) {
	if got := scaleChildTimeout(scaleSpec{Name: "combined"}); got != "70m" {
		t.Fatalf("combined child timeout %s, want 70m", got)
	}
	if got := scaleChildTimeout(scaleSpec{Name: "movies"}); got != "45m" {
		t.Fatalf("existing scale child timeout %s, want 45m", got)
	}
}

func TestScaleCombinedResultJSONContractIncludesSuccessfulZeroValues(t *testing.T) {
	result := scaleResult{
		Name:                  "combined",
		Titles:                10000,
		Items:                 55000,
		Feeds:                 20,
		RescanFolders:         2,
		RescanFiles:           29000,
		RescanFound:           2,
		RescanUnreachable:     0,
		IdleSeconds:           1800,
		LiveClients:           10,
		LiveRescanEvents:      10,
		BrowserTabs:           10,
		BrowserLiveStreams:    10,
		FullTextStatus:        "available: rebuilt and queried",
		FullTextRebuildMillis: 123,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"name":                 "combined",
		"titles":               float64(10000),
		"items":                float64(55000),
		"feeds":                float64(20),
		"rescan_folders":       float64(2),
		"rescan_files":         float64(29000),
		"rescan_found":         float64(2),
		"rescan_unreachable":   float64(0),
		"idle_seconds":         float64(1800),
		"live_clients":         float64(10),
		"live_rescan_events":   float64(10),
		"browser_tabs":         float64(10),
		"browser_live_streams": float64(10),
		"full_text_status":     "available: rebuilt and queried",
		"full_text_rebuild_ms": float64(123),
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("combined result %s=%v, want %v; JSON %s", key, got[key], value, raw)
		}
	}
}

func TestScaleBrowserRescanProofWaitsForAllRenderedTabs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envScaleBrowserDir, dir)
	t.Setenv(envScaleBrowserAddr, "127.0.0.1:1")
	written := make(chan error, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		raw, err := json.Marshal(scaleBrowserProof{Status: "ready", Tabs: 10, LiveStreams: 10, LiveRescanEvents: 10})
		if err == nil {
			temporary := filepath.Join(dir, "rescan-events-ready.json.tmp")
			err = os.WriteFile(temporary, raw, 0o644)
			if err == nil {
				err = os.Rename(temporary, filepath.Join(dir, "rescan-events-ready.json"))
			}
		}
		written <- err
	}()
	started := time.Now()
	proof := waitScaleBrowserRescanProof(t, time.Second)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 20*time.Millisecond {
		t.Fatal("rescan proof wait returned before the browser wrote its atomic proof")
	}
	if !validScaleBrowserRescanProof(proof) {
		t.Fatalf("rescan proof %+v was not accepted", proof)
	}
}

func TestScaleBrowserRescanProofRequiresEveryRenderedTabStreamAndEvent(t *testing.T) {
	valid := scaleBrowserProof{Status: "ready", Tabs: 10, LiveStreams: 10, LiveRescanEvents: 10}
	if !validScaleBrowserRescanProof(valid) {
		t.Fatal("complete rendered rescan proof was rejected")
	}
	for name, proof := range map[string]scaleBrowserProof{
		"status":  {Status: "failed", Tabs: 10, LiveStreams: 10, LiveRescanEvents: 10},
		"tabs":    {Status: "ready", Tabs: 9, LiveStreams: 10, LiveRescanEvents: 10},
		"streams": {Status: "ready", Tabs: 10, LiveStreams: 9, LiveRescanEvents: 10},
		"events":  {Status: "ready", Tabs: 10, LiveStreams: 10, LiveRescanEvents: 9},
	} {
		t.Run(name, func(t *testing.T) {
			if validScaleBrowserRescanProof(proof) {
				t.Fatalf("incomplete rendered rescan proof %+v was accepted", proof)
			}
		})
	}
}
