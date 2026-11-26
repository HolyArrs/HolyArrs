//go:build lab

package downloads

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// #861: a plain one-file movie job on a real SABnzbd. The first read of the
// finished job happens while its folder is not visible here yet (a network
// share that shows it a moment later): it finds no file, and the import waits
// and tries again on its own. The read after the folder shows up finds the
// file under the job's folder, with the save folder one level above it.
func TestLabSabnzbdLateFolder(t *testing.T) {
	base, key := cmpOr(os.Getenv("HOLYARRS_LAB_SAB_URL"), os.Getenv("HOLYARRS_LAB_SAB")), os.Getenv("HOLYARRS_LAB_SAB_KEY")
	news, disk := os.Getenv("HOLYARRS_LAB_NEWS_HOST"), os.Getenv("HOLYARRS_LAB_DOWNLOADS")
	if base == "" || key == "" || news == "" || disk == "" {
		t.Skip("lab SABnzbd settings not set")
	}
	name := "Lab.Late.Movie.2024.1080p.WEB.H264-HOLYARRS"
	nntp := startFakeNNTP(t, ":1119")
	data := make([]byte, 900_000)
	_, _ = rand.Read(data)
	nzb := nntp.makeNZB(name+".mkv", data, 200_000)
	ln, err := net.Listen("tcp", ":18797")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-nzb")
		_, _ = io.WriteString(w, nzb)
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	raw := func(mode string, q url.Values) string {
		q.Set("mode", mode)
		q.Set("output", "json")
		q.Set("apikey", key)
		resp, err := http.Get(base + "/api?" + q.Encode())
		if err != nil {
			t.Fatalf("%s: %v", mode, strings.ReplaceAll(err.Error(), key, "KEY"))
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return strings.ReplaceAll(string(b), key, "KEY")
	}
	server := "holyarrs-lab-fake-news"
	raw("set_config", url.Values{"section": {"servers"}, "keyword": {server}, "host": {news}, "port": {"1119"}, "connections": {"2"},
		"ssl": {"0"}, "enable": {"1"}, "retention": {"0"}})
	defer raw("del_config", url.Values{"section": {"servers"}, "keyword": {server}})

	ctx := context.Background()
	s := NewSabnzbd(base, key, nil)
	if old, err := s.List(ctx, ""); err == nil {
		for _, o := range old {
			if strings.HasPrefix(o.Name, "Lab.Late.") {
				_ = s.Remove(ctx, o.ID, true)
			}
		}
	}
	id, err := s.Add(ctx, AddRequest{URL: "http://" + news + ":18797/late.nzb", Category: "movies", Name: name})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	defer func() { _ = s.Remove(context.Background(), id, true) }()
	var j Job
	deadline := time.Now().Add(4 * time.Minute)
	for {
		j, err = s.Get(ctx, id)
		if err == nil && (j.State == StateFinished && j.ContentPath != "" || j.State == StateFailed) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not done: %+v %v", j, err)
		}
		time.Sleep(time.Second)
	}
	if j.State != StateFinished {
		t.Fatalf("failed: %s", j.Detail)
	}
	h := raw("history", url.Values{"category": {"movies"}, "limit": {"3"}})
	if i := strings.Index(h, `"storage":`); i >= 0 {
		t.Logf("SABnzbd storage: %s", h[i:i+strings.Index(h[i:], ",")])
	}
	t.Logf("job %s: save %s, content %s", j.Name, j.SavePath, j.ContentPath)

	// The folder is not visible here yet: an empty share in place of the real one.
	early := t.TempDir()
	pm := []domain.PathMap{{Theirs: "/data/downloads", Ours: filepath.ToSlash(early)}}
	// A job folder reads as no file; a job SABnzbd stored as one file falls
	// back to its name, and the import waits for that file to arrive.
	save, seen := readFinished(j, pm, early)
	_ = save
	if folder := !strings.HasSuffix(j.ContentPath, ".mkv"); folder && len(seen) != 0 || !folder && len(seen) != 1 {
		t.Fatalf("early read found %v", seen)
	}
	if !importer.RetriesOnItsOwn(importer.NotPresentYetWhy) {
		t.Fatal("an empty read is not tried again on its own")
	}

	// The share shows the folder now: the job's folder lands in it.
	late := t.TempDir()
	theirs := parentDir(j.SavePath)
	rel := strings.TrimPrefix(j.ContentPath, theirs)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(late, filepath.FromSlash(rel))), 0o755); err != nil {
		t.Fatal(err)
	}
	if c := os.Getenv("HOLYARRS_LAB_SAB_CONTAINER"); c != "" {
		if b, err := exec.Command("docker", "cp", c+":"+j.ContentPath, filepath.Join(late, filepath.FromSlash(rel))).CombinedOutput(); err != nil {
			t.Fatalf("docker cp: %v %s", err, b)
		}
	} else {
		late, theirs = disk, "/data/downloads"
	}
	pm = []domain.PathMap{{Theirs: theirs, Ours: filepath.ToSlash(late)}}
	save, seen = readFinished(j, pm, late)
	t.Logf("late read: save %s, files %+v", save, seen)
	if save != j.SavePath || len(seen) != 1 || !strings.HasSuffix(seen[0].Name, name+".mkv") || seen[0].Size != int64(len(data)) {
		t.Fatalf("late read save %s files %+v", save, seen)
	}
	if got := filepath.Join(mapLocal(save, pm), filepath.FromSlash(seen[0].Name)); !fileThere(got) {
		t.Fatalf("save %s + %s is not the file", save, seen[0].Name)
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func fileThere(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
