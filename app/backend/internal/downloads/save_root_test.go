package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// NZBGet keeps finished jobs in its main folder, in a category's own folder,
// or in the main one with the category's name added.
func TestNZBGetTellsItsFinishedFolders(t *testing.T) {
	f := &fakeNZBGet{user: "u", pass: "p", config: []nzbParam{
		{Name: "MainDir", Value: "/data/nzbget"}, {Name: "DestDir", Value: "${MainDir}/completed"}, {Name: "AppendCategoryDir", Value: "yes"},
		{Name: "Category1.Name", Value: "Movies"}, {Name: "Category1.DestDir", Value: ""},
		{Name: "Category2.Name", Value: "tv"}, {Name: "Category2.DestDir", Value: "/shows/tv"},
		{Name: "Category3.Name", Value: "music"}, {Name: "Category3.DestDir", Value: "/music"},
	}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	got, err := NewNZBGet(srv.URL, "u", "p", nil).finishedFolders(context.Background(), []string{"movies", "tv"})
	want := []string{"/data/nzbget/completed", "/data/nzbget/completed/Movies", "/shows/tv"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("folders %v %v, want %v", got, err, want)
	}
}

// SABnzbd gives its completed folder as a full path in fullstatus; a
// category folder counts only when it is a full path of its own.
func TestSabnzbdTellsItsFinishedFolders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("apikey") != sabKey:
			w.WriteHeader(http.StatusForbidden)
		case q.Get("mode") == "fullstatus" && q.Get("skip_dashboard") == "1":
			_, _ = w.Write([]byte(`{"status": {"completedir": "/data/usenet/complete", "downloaddir": "/data/usenet/incomplete"}}`))
		case q.Get("mode") == "get_config" && q.Get("section") == "categories":
			_, _ = w.Write([]byte(`{"config": {"categories": [{"name": "*", "dir": ""}, {"name": "movies", "dir": "/films/*"}, {"name": "tv", "dir": "tv"}, {"name": "audio", "dir": "/audio"}]}}`))
		default:
			_, _ = w.Write([]byte(`{"status": false, "error": "not implemented"}`))
		}
	}))
	t.Cleanup(srv.Close)
	got, err := NewSabnzbd(srv.URL, sabKey, nil).finishedFolders(context.Background(), []string{"Movies", "tv"})
	want := []string{"/data/usenet/complete", "/films/"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("folders %v %v, want %v", got, err, want)
	}
}

type saveRootRig struct {
	srv *httptest.Server
	s   *database.Store
	v   *secrets.Vault
	tmp string
	cfg string
}

func newSaveRootRig(t *testing.T) saveRootRig {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "config")
	a := &Web{Store: s, Metadata: metadata.Lab(), Secrets: v, Downloads: &Manager{Store: s, FromVault: v.Get, ConfigDir: cfg}}
	return saveRootRig{srv: webRig(t, a), s: s, v: v, tmp: tmp, cfg: cfg}
}

// nzbget starts a fake NZBGet whose finished jobs go to destDir.
func (r saveRootRig) nzbget(t *testing.T, destDir string) string {
	t.Helper()
	f := &fakeNZBGet{user: "u", pass: "p", config: []nzbParam{{Name: "MainDir", Value: filepath.Dir(destDir)}, {Name: "DestDir", Value: destDir},
		{Name: "AppendCategoryDir", Value: "yes"}}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return srv.URL
}

func (r saveRootRig) add(t *testing.T, name, url string) DownloaderView {
	t.Helper()
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", r.srv.URL+"/api/v1/downloaders", body(map[string]any{"name": name, "kind": "nzbget", "url": url,
		"username": "u", "secret": "p", "category": "movies", "allow_local_network": true}), 201), &d)
	return d
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o750); err != nil {
		t.Fatal(err)
	}
	return p
}

// With no save path and no path pair a finished job cannot be read, so the
// owner's add keeps the Downloader's own finished folder, when HolyArrs sees
// it at the same path, as its save root. Imports then read the job there.
func TestOwnersAddKeepsTheDownloadersFinishedFolder(t *testing.T) {
	r := newSaveRootRig(t)
	done := mkdir(t, filepath.Join(r.tmp, "nzbget", "completed"))
	job := mkdir(t, filepath.Join(done, "Arcane.S01E01"))
	if err := os.WriteFile(filepath.Join(job, "a.mkv"), []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := r.add(t, "NZBGet", r.nzbget(t, done))
	if want := []domain.PathMap{{Theirs: done, Ours: done}}; !slices.Equal(d.PathMap, want) {
		t.Fatalf("path pairs %+v, want %+v", d.PathMap, want)
	}
	if _, files := readFinished(Job{ContentPath: job, SavePath: done}, d.PathMap, ""); len(files) != 1 {
		t.Fatalf("finished job read %+v", files)
	}

	// A folder HolyArrs does not see, a whole disk and HolyArrs's own config
	// folder are never kept.
	for i, dest := range []string{filepath.Join(r.tmp, "elsewhere"), filepath.VolumeName(r.tmp) + string(filepath.Separator), mkdir(t, filepath.Join(r.cfg, "completed")), r.cfg} {
		if d := r.add(t, "NZBGet "+itoa(int64(i)), r.nzbget(t, dest)); len(d.PathMap) != 0 {
			t.Fatalf("%s kept: %+v", dest, d.PathMap)
		}
	}
}

// A Downloader saved before has no save root: the owner's next Test or save
// fills it once. A path pair the owner removes in an edit stays removed.
func TestOwnersTestOrSaveFillsTheSaveRootOfAnOldDownloader(t *testing.T) {
	r := newSaveRootRig(t)
	done := mkdir(t, filepath.Join(r.tmp, "nzbget", "completed"))
	url := r.nzbget(t, done)
	for _, way := range []string{"test", "save"} {
		cat := "movies"
		user, allow := "u", true
		old, err := Add(context.Background(), r.s, r.v, Input{Name: "Old " + way, Adapter: "nzbget", URL: url, Username: &user, Secret: "p", Category: &cat, AllowLocal: &allow}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if len(old.PathMap) != 0 {
			t.Fatalf("Add by itself kept %+v", old.PathMap)
		}
		id := itoa(old.ID)
		if way == "test" {
			var res testAnswer
			_ = json.Unmarshal(do(t, "POST", r.srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
			if !res.OK || !strings.Contains(res.Text, done) {
				t.Fatalf("test %+v", res)
			}
		} else {
			do(t, "PUT", r.srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "Old save", "path_map": []any{}}), 200)
		}
		var d domain.Downloader
		if err := database.Get(context.Background(), r.s.Read(), "downloaders", old.ID, &d); err != nil {
			t.Fatal(err)
		}
		if want := []domain.PathMap{{Theirs: done, Ours: done}}; !slices.Equal(d.PathMap, want) {
			t.Fatalf("%s: path pairs %+v, want %+v", way, d.PathMap, want)
		}
		var v DownloaderView
		_ = json.Unmarshal(do(t, "PUT", r.srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "Old " + way, "path_map": []any{}}), 200), &v)
		if len(v.PathMap) != 0 {
			t.Fatalf("%s: the owner's removal was undone: %+v", way, v.PathMap)
		}
	}
}
