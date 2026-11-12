//go:build lab

package downloads

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestLabAria2 runs against a disposable aria2:
//
//	HOLYARRS_LAB_ARIA2             the JSON-RPC address, e.g. http://127.0.0.1:6800/jsonrpc
//	HOLYARRS_LAB_ARIA2_SECRET_FILE a file holding its rpc-secret, when it has one
//	HOLYARRS_LAB_ARIA2_THEIRS      aria2's download folder (default /downloads)
//	HOLYARRS_LAB_ARIA2_HERE        the same folder as this test sees it
//
// Payloads are placed in the folder first and aria2's check-integrity is
// turned on for the run, so torrents finish by checking the files, with no
// peers. Every torrent is private and has no tracker but a closed local port.
func TestLabAria2(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_ARIA2")
	here := os.Getenv("HOLYARRS_LAB_ARIA2_HERE")
	if base == "" || here == "" {
		t.Skip("HOLYARRS_LAB_ARIA2 and HOLYARRS_LAB_ARIA2_HERE are not set")
	}
	theirs := os.Getenv("HOLYARRS_LAB_ARIA2_THEIRS")
	if theirs == "" {
		theirs = "/downloads"
	}
	secret := ""
	if f := os.Getenv("HOLYARRS_LAB_ARIA2_SECRET_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		secret = strings.TrimSpace(string(b))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	const category = "holyarrs-labtest-aria2"
	e := NewAria2(base, secret, Client(base, true))
	e.Category = category
	e.PathMap = []domain.PathMap{{Theirs: theirs, Ours: here}}
	e.Keep = func(context.Context) ([]string, error) { return []string{"/data/media"}, nil }
	folder := filepath.Join(here, category)
	if err := os.MkdirAll(folder, 0o777); err != nil {
		t.Fatal(err)
	}

	h, err := e.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	t.Logf("aria2 %s; note: %s", h.Version, h.Note)
	if h.Version != "1.36.0" {
		t.Errorf("version %q", h.Version)
	}
	if secret != "" {
		if _, err := NewAria2(base, "wrong", Client(base, true)).Health(ctx); !errors.Is(err, ErrAuth) {
			t.Errorf("wrong secret: %v", err)
		}
	}

	var before map[string]string
	if err := e.call(ctx, "aria2.getGlobalOption", &before); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		put := map[string]string{}
		for _, k := range []string{"check-integrity", "max-overall-download-limit", "max-overall-upload-limit"} {
			put[k] = before[k]
		}
		if err := e.call(context.Background(), "aria2.changeGlobalOption", nil, put); err != nil {
			t.Errorf("put back options: %v", err)
		}
		jobs, _ := e.List(context.Background(), category)
		for _, j := range jobs {
			if err := e.Remove(context.Background(), j.ID, false); err != nil {
				t.Errorf("cleanup %s: %v", j.ID, err)
			}
		}
		rows, _ := e.all(context.Background())
		for _, s := range rows {
			if a2Folder(s.Dir) == category {
				_ = e.forget(context.Background(), s)
			}
		}
		_ = os.RemoveAll(folder)
	})
	if err := e.call(ctx, "aria2.changeGlobalOption", nil, map[string]string{"check-integrity": "true"}); err != nil {
		t.Fatal(err)
	}

	finished := func(name string) (string, AddRequest) {
		t.Helper()
		payload := []byte("HolyArrs disposable aria2 acceptance " + name + "\n")
		// The image's stop script may remove an empty folder a moment after an
		// earlier run's cleanup, so the folder is made again right before use.
		if err := os.MkdirAll(folder, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, name), payload, 0o666); err != nil {
			t.Fatal(err)
		}
		torrent := a2LabTorrent(name, payload)
		hash, _ := torrentFileHash(torrent)
		src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(torrent) }))
		t.Cleanup(src.Close)
		return hash, AddRequest{URL: src.URL + "/lab.torrent", Name: name, Category: category, InfoHash: hash, AllowLocal: true, SavePath: "/data/downloads"}
	}
	jobCount := func() int64 {
		var st map[string]string
		if err := e.call(ctx, "aria2.getGlobalStat", &st); err != nil {
			t.Fatal(err)
		}
		return a2Num(st["numActive"]) + a2Num(st["numWaiting"]) + a2Num(st["numStopped"])
	}
	waitDone := func(id string) Job {
		t.Helper()
		var j Job
		var err error
		for ctx.Err() == nil {
			if j, err = e.Get(ctx, id); err == nil && j.State == StateFinished {
				return j
			}
			time.Sleep(300 * time.Millisecond)
		}
		t.Fatalf("not finished: %+v %v", j, err)
		return j
	}

	name := "HolyArrs.Lab.aria2.Acceptance.2026.mkv"
	hash, req := finished(name)
	n0 := jobCount()
	id, err := e.Add(ctx, req)
	if err != nil || id != hash {
		t.Fatalf("add: %q %v", id, err)
	}
	if again, err := e.Add(ctx, req); err != nil || again != id {
		t.Errorf("second add: %q %v", again, err)
	}
	if n := jobCount(); n != n0+1 {
		t.Errorf("jobs %d after two adds, want %d", n, n0+1)
	}
	if got, err := e.Add(ctx, AddRequest{InfoHash: hash, Category: category, ReconcileOnly: true}); err != nil || got != id {
		t.Errorf("reconcile: %q %v", got, err)
	}
	if jobs, err := e.List(ctx, "tv-sonarr"); err != nil || hasJob(jobs, id) {
		t.Errorf("another folder lists it: %v", err)
	}
	j := waitDone(id)
	if j.Progress != 1 || len(j.Files) != 1 || j.Files[0].Name != name || j.Category != category || j.ContentPath != theirs+"/"+category+"/"+name {
		t.Fatalf("finished: %+v", j)
	}
	t.Logf("finished: %s %s", j.State, j.Detail)

	// A private torrent that has not finished: trackers, pause and resume.
	unfinished := "HolyArrs.Lab.aria2.Private.2026.mkv"
	ptorrent := a2LabTorrent(unfinished, []byte("never on disk\n"))
	phash, _ := torrentFileHash(ptorrent)
	pid, err := e.Add(ctx, AddRequest{File: ptorrent, Category: category, InfoHash: phash, AllowLocal: true})
	if err != nil || pid != phash {
		t.Fatalf("add private: %q %v", pid, err)
	}
	ps, _, _, err := e.find(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	var trackers struct {
		Bittorrent struct {
			AnnounceList [][]string `json:"announceList"`
		} `json:"bittorrent"`
	}
	if err := e.call(ctx, "aria2.tellStatus", &trackers, ps.GID, []string{"bittorrent"}); err != nil {
		t.Fatal(err)
	}
	if l := trackers.Bittorrent.AnnounceList; len(l) != 1 {
		t.Errorf("private torrent trackers: %v", l)
	}
	status := func(gid string) string {
		var s a2Status
		_ = e.call(ctx, "aria2.tellStatus", &s, gid, []string{"status"})
		return s.Status
	}
	if err := e.PauseItem(ctx, pid); err != nil || status(ps.GID) != "paused" {
		t.Errorf("pause: %v %s", err, status(ps.GID))
	}
	if j, _ := e.Get(ctx, pid); j.State != StatePaused {
		t.Errorf("paused job reads %s %s", j.State, j.Detail)
	}
	if err := e.ResumeItem(ctx, pid); err != nil || status(ps.GID) == "paused" {
		t.Errorf("resume: %v %s", err, status(ps.GID))
	}
	if err := e.Remove(ctx, pid, false); err != nil {
		t.Errorf("remove unfinished: %v", err)
	}
	if _, err := e.Get(ctx, pid); !errors.Is(err, ErrNotFound) {
		t.Errorf("after remove: %v", err)
	}

	// A magnet shows as metadata. It is added paused so it never looks for peers.
	mhash := "a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2"
	var mgid string
	if err := e.call(ctx, "aria2.addUri", &mgid, []string{"magnet:?xt=urn:btih:" + mhash + "&dn=HolyArrs.Lab.Magnet"},
		map[string]string{"dir": theirs + "/" + category, "pause": "true"}); err != nil {
		t.Fatal(err)
	}
	if j, err := e.Get(ctx, mhash); err != nil || j.State != StateDownloading || j.Detail != "metadata" || j.Name != "HolyArrs.Lab.Magnet" {
		t.Errorf("magnet: %+v %v", j, err)
	}
	if err := e.Remove(ctx, mhash, false); err != nil {
		t.Errorf("remove magnet: %v", err)
	}

	// Caps round trip.
	st0, err := e.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetSpeedLimit(ctx, 1048576, 524288); err != nil {
		t.Fatal(err)
	}
	if st, err := e.Status(ctx); err != nil || st.SpeedLimit != 1048576 || st.UpLimit != 524288 {
		t.Errorf("caps: %+v %v", st, err)
	}
	if err := e.SetSpeedLimit(ctx, st0.SpeedLimit, st0.UpLimit); err != nil {
		t.Errorf("put back caps: %v", err)
	}

	// Remove with files keeps a file aria2 took over by checking it, since it
	// was on disk before the add.
	local := filepath.Join(folder, name)
	keepName := "HolyArrs.Lab.aria2.Kept.2026.mkv"
	khash, kreq := finished(keepName)
	if _, err := e.Add(ctx, kreq); err != nil {
		t.Fatal(err)
	}
	waitDone(khash)
	if err := e.Remove(ctx, id, true); err != nil {
		t.Fatalf("remove with files: %v", err)
	}
	if _, err := e.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("after remove: %v", err)
	}
	if _, err := os.Stat(local); err != nil {
		t.Errorf("a file aria2 took over was deleted: %v", err)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Errorf("the category folder went too: %v", err)
	}
	if err := e.Remove(ctx, khash, false); err != nil {
		t.Fatalf("remove without files: %v", err)
	}
	if _, err := os.Stat(filepath.Join(folder, keepName)); err != nil {
		t.Errorf("a remove without files deleted them: %v", err)
	}
	if jobs, _ := e.List(ctx, category); len(jobs) != 0 {
		t.Errorf("left %d jobs", len(jobs))
	}
}

// a2LabTorrent is a private one-file torrent for name holding payload. Its
// only tracker is a closed local port.
func a2LabTorrent(name string, payload []byte) []byte {
	piece := sha1.Sum(payload)
	info := fmt.Sprintf("d6:lengthi%de4:name%d:%s12:piece lengthi16384e6:pieces20:%s7:privatei1ee", len(payload), len(name), name, piece[:])
	announce := "http://127.0.0.1:9/announce"
	return []byte(fmt.Sprintf("d8:announce%d:%s4:info%se", len(announce), announce, info))
}
