//go:build lab

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// on the lab, from a throwaway container in the lab network
// with research/lab/downloads at /data/downloads:
//
//	HOLYARRS_LAB139_OUT=/src/app/frontend/src/lib/requests/testdata/lab139.json go test -tags lab -run TestLabAskLiveProgress ./app/backend/cmd/holyarrs/
//
// A member asks for Dune: Part Two, the owner approves it (no sign-in),
// and the app does the rest as it does for real: add, search the lab fake
// indexer, grab our own Dune test torrent into the lab qBittorrent, import,
// and mark the ask Ready straight after the import.
// All the while the test reads GET /api/v1/searches and
// GET /api/v1/downloads/view, as the Requests page does, and keeps each
// change. The source is reached through a small local proxy that holds
// each search for two seconds, so the search can be seen running, and
// gives the test file's real size. The
// kept responses feed app/frontend/src/lib/requests/live.lab.test.ts.
func TestLabAskLiveProgress(t *testing.T) {
	const hash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	const cat = "holyarrs-lab139"
	indexer := os.Getenv("HOLYARRS_LAB_SOURCE")
	if indexer == "" {
		indexer = "http://fake-indexer/api"
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		u := indexer
		if r.URL.RawQuery != "" {
			u += "?" + r.URL.RawQuery
		}
		resp, err := http.Get(u)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		// The fake indexer lists our 28 MB Dune test file as 8.4 GB; the
		// importer rightly calls that a fake, so the proxy gives its real size.
		b = bytes.ReplaceAll(b, []byte("8400000000"), []byte("29588180"))
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(b)
	}))
	t.Cleanup(slow.Close)

	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	movies := t.TempDir()
	var ana int64
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: movies, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System); err != nil {
			return err
		}
		var err error
		if ana, err = database.Insert(tx, "users", &domain.User{Name: "ana", Role: "member", State: "active"}, domain.System); err != nil {
			return err
		}
		so := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: slow.URL + "/api", AllowLocal: true,
			UseFor: domain.SourceUse{AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"}, Categories: map[string][]int{"movie": {2000}},
			Priority: 25, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", so, domain.System); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: labQbit(), Category: cat, Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err = database.Insert(tx, "downloaders", d, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wired as cmd/holyarrs wires it.
	ctx, cancel := context.WithCancel(context.Background())
	q := jobs.New(s, jobs.Options{Workers: 2, BaseBackoff: time.Hour, PollInterval: 50 * time.Millisecond})
	md := metadata.Lab()
	lib := &library.Service{Store: s, Queue: q, Metadata: md}
	m := &downloads.Manager{Store: s, SavePath: filepath.Join("/data/downloads/incomplete", cat)}
	im := &importer.Importer{Store: s}
	sr := &sources.Searcher{Store: s, Queue: q, OnChosen: func(ctx context.Context, slotID, releaseID int64) {
		_, _, _ = q.Enqueue(ctx, downloads.KindGrab, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID}, fmt.Sprintf("grab:%d", slotID), domain.System)
	}}
	m.OnFinished = func(ctx context.Context, id int64) {
		_, _, _ = q.Enqueue(ctx, importer.KindImport, &domain.Subject{Type: "download", ID: id}, nil, fmt.Sprintf("import:%d", id), domain.System)
	}
	sr.Register(q)
	lib.Register(q)
	q.Handle(downloads.KindGrab, m.HandleGrab)
	svc := &requests.Service{Store: s, Library: lib, Metadata: md}
	// As cmd/holyarrs: the ready check runs right after each import.
	q.Handle(importer.KindImport, func(ctx context.Context, r *jobs.Run) error {
		err := im.HandleJob(ctx, r)
		if err == nil {
			_, _ = svc.CheckReady(ctx)
		}
		return err
	})
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); q.Stop() })
	labRemoveCat(t, m, cat, hash)
	// Our own Dune test file goes in the lab downloads folder first, the
	// way the 4.14 tests do it, so the lab qBittorrent finishes from what
	// it already has even when it cannot reach qbt-seed.
	payload := os.Getenv("HOLYARRS_LAB_PAYLOAD")
	if payload == "" {
		payload = "/seed/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"
	}
	// Only the first half goes in at first, so the row shows a real part
	// way; the rest follows once that was seen, and qBittorrent rechecks.
	full, perr := os.ReadFile(payload)
	put := func(b []byte) {
		dir := filepath.Join("/data/downloads/incomplete", cat)
		if err := os.MkdirAll(dir, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(payload)), b, 0o666); err != nil {
			t.Fatal(err)
		}
	}
	rest := func() {
		put(full)
		resp, err := http.PostForm(labQbit()+"/api/v2/torrents/recheck", url.Values{"hashes": {hash}})
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if perr == nil {
		put(full[:len(full)/2])
	} else {
		t.Logf("no payload at %s: the lab qBittorrent must fetch it from qbt-seed", payload)
	}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: md, Version: "lab", Downloads: m, Queue: q, Library: lib, Asks: svc}), config{}))
	t.Cleanup(srv.Close)

	res, err := svc.Ask(ctx, requests.Person{UserID: ana, Name: "ana"}, "693134")
	if err != nil || res.Ask.State != domain.AskAsked {
		t.Fatalf("ask %+v %v", res.Ask, err)
	}
	do(t, "POST", srv.URL+"/api/v1/asks/"+labItoa(res.Ask.ID)+"/approve", `{}`, 200)

	type snap struct {
		At        string          `json:"at"`
		Searches  json.RawMessage `json:"searches"`
		Downloads json.RawMessage `json:"downloads"`
		Ask       map[string]any  `json:"ask"`
		search    []sources.SearchView
		rows      []downloads.DownloadRow
	}
	var snaps []snap
	key := ""
	take := func() snap {
		var x snap
		x.Searches = do(t, "GET", srv.URL+"/api/v1/searches", "", 200)
		x.Downloads = do(t, "GET", srv.URL+"/api/v1/downloads/view", "", 200)
		var sv struct {
			Running []sources.SearchView `json:"running"`
		}
		_ = json.Unmarshal(x.Searches, &sv)
		var dv struct {
			Downloads []downloads.DownloadRow `json:"downloads"`
		}
		_ = json.Unmarshal(x.Downloads, &dv)
		x.search, x.rows = sv.Running, dv.Downloads
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/asks/"+labItoa(res.Ask.ID), "", 200), &x.Ask)
		return x
	}
	sawSearch, sawDownload, sawImport, needRest := false, false, false, false
	deadline := time.Now().Add(4 * time.Minute)
	for {
		_, _ = m.SyncOnce(ctx)
		x := take()
		k := fmt.Sprint(x.Ask["state"], " ", len(x.search))
		for _, r := range x.search {
			k += r.Kind + r.State
		}
		done := false
		for _, r := range x.rows {
			k += fmt.Sprintf("|%s %.2f", r.Phase, r.Progress)
			switch r.Phase {
			case "waiting", "downloading", "stalled", "paused":
				if r.Progress > 0 && r.Progress < 1 && !sawDownload {
					sawDownload = true
					needRest = perr == nil
				}
			case "waiting_to_import", "importing":
				sawImport = true
			case "done":
				done = x.Ask["state"] == domain.AskAvailable
			case "failed", "vanished", "import_refused", "needs_a_decision":
				var js []string
				if rows, err := s.Read().Query("SELECT kind || ' ' || state || ' ' || COALESCE(result, '') || ' ' || COALESCE(error, '') FROM jobs WHERE kind = ? ORDER BY id", importer.KindImport); err == nil {
					for rows.Next() {
						var j string
						_ = rows.Scan(&j)
						js = append(js, j)
					}
					rows.Close()
				}
				t.Fatalf("download %s: %+v; imports %v", r.Phase, r, js)
			}
		}
		for _, r := range x.search {
			if r.Kind != jobs.KindFeedCheck && r.TitleID != nil {
				sawSearch = true
			}
		}
		if k != key {
			key = k
			x.At = time.Now().UTC().Format(time.RFC3339Nano)
			snaps = append(snaps, x)
			t.Logf("%s", k)
		}
		if needRest {
			needRest = false
			rest()
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out; last %s", key)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !sawSearch || !sawDownload || !sawImport {
		t.Errorf("saw searching %v, downloading %v, importing %v", sawSearch, sawDownload, sawImport)
	}
	if out := os.Getenv("HOLYARRS_LAB139_OUT"); out != "" {
		b, _ := json.MarshalIndent(snaps, "", "\t")
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// labRemoveCat removes the lab torrents and the category folder afterwards.
func labRemoveCat(t *testing.T, m *downloads.Manager, cat string, hashes ...string) {
	t.Cleanup(func() {
		if e, err := m.EngineFor(context.Background(), domain.Downloader{Adapter: "qbittorrent", URL: labQbit(), AllowLocal: true}); err == nil {
			for _, h := range hashes {
				_ = e.Remove(context.Background(), h, true)
			}
		}
		_ = os.RemoveAll(filepath.Join("/data/downloads/incomplete", cat))
	})
}
