//go:build lab

package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestLabRTorrent runs against a disposable rTorrent:
//
//	HOLYARRS_LAB_RTORRENT        the XML-RPC address, e.g. http://127.0.0.1:8000/RPC2
//	HOLYARRS_LAB_RTORRENT_USER   and HOLYARRS_LAB_RTORRENT_PASSWORD_FILE when it asks for a sign-in
//	HOLYARRS_LAB_RTORRENT_THEIRS rTorrent's download folder (default /downloads)
//	HOLYARRS_LAB_RTORRENT_HERE   the same folder as this test sees it
//
// The payload is placed in rTorrent's default folder first, so the torrent
// finishes by checking the files, with no peers and no tracker.
func TestLabRTorrent(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_RTORRENT")
	here := os.Getenv("HOLYARRS_LAB_RTORRENT_HERE")
	if base == "" || here == "" {
		t.Skip("HOLYARRS_LAB_RTORRENT and HOLYARRS_LAB_RTORRENT_HERE are not set")
	}
	theirs := os.Getenv("HOLYARRS_LAB_RTORRENT_THEIRS")
	if theirs == "" {
		theirs = "/downloads"
	}
	pass := ""
	if f := os.Getenv("HOLYARRS_LAB_RTORRENT_PASSWORD_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		pass = strings.TrimSpace(string(b))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	const category = "holyarrs labtest"
	e := NewRTorrent(base, os.Getenv("HOLYARRS_LAB_RTORRENT_USER"), pass, Client(base, true))
	e.Category = category
	e.PathMap = []domain.PathMap{{Theirs: theirs, Ours: here}}
	e.Keep = func(context.Context) ([]string, error) { return []string{"/data/media"}, nil }

	h, err := e.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	t.Logf("rTorrent %s", h.Version)
	if os.Getenv("HOLYARRS_LAB_RTORRENT_USER") != "" {
		bad := NewRTorrent(base, os.Getenv("HOLYARRS_LAB_RTORRENT_USER"), "wrong", Client(base, true))
		if _, err := bad.Health(ctx); !errors.Is(err, ErrAuth) {
			t.Errorf("wrong password: %v", err)
		}
	}

	name := "HolyArrs.Lab.rTorrent.Acceptance.2026.mkv"
	payload := []byte("HolyArrs disposable rTorrent acceptance\n")
	local := filepath.Join(here, "temp", name)
	if err := os.MkdirAll(filepath.Dir(local), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, payload, 0o666); err != nil {
		t.Fatal(err)
	}
	torrent := rtTorrentFile(name, payload)
	hash, _ := torrentFileHash(torrent)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(torrent) }))
	defer src.Close()
	req := AddRequest{URL: src.URL + "/lab.torrent", Name: name, Category: category, InfoHash: hash, AllowLocal: true, SavePath: "/data/downloads"}

	id, err := e.Add(ctx, req)
	if err != nil || id != hash {
		t.Fatalf("add: %q %v", id, err)
	}
	again, err := e.Add(ctx, req)
	if err != nil || again != id {
		t.Errorf("second add: %q %v", again, err)
	}
	if got, err := e.Add(ctx, AddRequest{InfoHash: hash, Category: category, ReconcileOnly: true}); err != nil || got != id {
		t.Errorf("reconcile: %q %v", got, err)
	}
	if jobs, err := e.List(ctx, category); err != nil || !hasJob(jobs, id) {
		t.Errorf("list: %v %v", jobs, err)
	}
	if jobs, err := e.List(ctx, "tv-sonarr"); err != nil || hasJob(jobs, id) {
		t.Errorf("another label lists it: %v", err)
	}

	var j Job
	for ctx.Err() == nil {
		if j, err = e.Get(ctx, id); err == nil && j.State == StateFinished {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if j.State != StateFinished || j.Progress != 1 || len(j.Files) != 1 || j.Files[0].Name != name || j.Category != category {
		t.Fatalf("finished: %+v %v", j, err)
	}
	if j.ContentPath != theirs+"/temp/"+name {
		t.Errorf("content path %q", j.ContentPath)
	}
	t.Logf("finished: %s %s ratio %.2f", j.State, j.Detail, j.Ratio)

	if err := e.PauseItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	if j, _ = e.Get(ctx, id); j.Detail != "complete" || j.ContentPath == "" {
		t.Errorf("paused: %+v", j)
	}
	if st, err := e.Status(ctx); err != nil || !st.Paused {
		t.Errorf("status while paused: %+v %v", st, err)
	}
	if err := e.ResumeItem(ctx, id); err != nil {
		t.Fatal(err)
	}
	if j, _ = e.Get(ctx, id); j.Detail != "seeding" {
		t.Errorf("resumed: %+v", j)
	}

	if err := e.Remove(ctx, id, true); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := e.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("after remove: %v", err)
	}
	if _, err := os.Stat(local); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the files were not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(here, "temp")); err != nil {
		t.Errorf("the save folder went too: %v", err)
	}
}

func hasJob(jobs []Job, id string) bool {
	for _, j := range jobs {
		if j.ID == id {
			return true
		}
	}
	return false
}
