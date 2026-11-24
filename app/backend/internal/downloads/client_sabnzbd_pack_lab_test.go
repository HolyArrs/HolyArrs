//go:build lab

package downloads

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A season pack whose RAR unpacks into one folder per episode, run through the
// lab SABnzbd from the host: every episode is read from the job and matched.
//
//	HOLYARRS_LAB_SAB_URL   SABnzbd address seen from here (http://127.0.0.1:18707)
//	HOLYARRS_LAB_SAB_KEY   its API key
//	HOLYARRS_LAB_NEWS_HOST this machine as SABnzbd sees it (host.docker.internal)
//	HOLYARRS_LAB_DOWNLOADS the host folder mounted as /data/downloads
//	HOLYARRS_LAB_RAR       rar program that builds the pack
func TestLabSabnzbdNestedPack(t *testing.T) {
	base, key := os.Getenv("HOLYARRS_LAB_SAB_URL"), os.Getenv("HOLYARRS_LAB_SAB_KEY")
	news, disk, rar := os.Getenv("HOLYARRS_LAB_NEWS_HOST"), os.Getenv("HOLYARRS_LAB_DOWNLOADS"), os.Getenv("HOLYARRS_LAB_RAR")
	if base == "" || key == "" || news == "" || disk == "" || rar == "" {
		t.Skip("lab SABnzbd pack settings not set")
	}
	pack := "Lab.Pack.S02.MULTi.1080p.WEB.AV1-HOLYARRS"
	src := t.TempDir()
	for e := 1; e <= 10; e++ {
		n := fmt.Sprintf("Lab.Pack.S02E%02d.MULTi.1080p.WEB.AV1-HOLYARRS", e)
		if e == 10 {
			n = "Lab.Pack.S02E10.FiNAL.MULTi.1080p.WEB.AV1-HOLYARRS"
		}
		b := make([]byte, 150_000)
		_, _ = rand.Read(b)
		p := filepath.Join(src, pack, n, n+".mkv")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	cmd := exec.Command(rar, "a", "-r", "-ma5", "-m0", "-v600k", "-ep1", "-idq", filepath.Join(out, pack+".rar"), filepath.Join(src, pack))
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rar: %v %s", err, b)
	}
	vols, _ := filepath.Glob(filepath.Join(out, "*.rar"))
	sort.Strings(vols)
	if len(vols) < 2 {
		t.Fatalf("volumes %v", vols)
	}

	nntp := startFakeNNTP(t, ":1119")
	var files strings.Builder
	for i, v := range vols {
		data, _ := os.ReadFile(v)
		one := nntp.makeNZB(filepath.Base(v), data, 200_000)
		s, e := strings.Index(one, "<file "), strings.Index(one, "</file>")
		files.WriteString(strings.Replace(one[s:e+len("</file>")], "[1/1]", fmt.Sprintf("[%d/%d]", i+1, len(vols)), 1))
	}
	nzb := `<?xml version="1.0" encoding="UTF-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">` + files.String() + `</nzb>
`
	ln, err := net.Listen("tcp", ":18798")
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
	id, err := s.Add(ctx, AddRequest{URL: "http://" + news + ":18798/pack.nzb", Category: "tv", Name: pack})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if os.Getenv("HOLYARRS_LAB_KEEP") != "1" {
		defer func() { _ = s.Remove(context.Background(), id, true) }()
	}
	var j Job
	deadline := time.Now().Add(4 * time.Minute)
	for {
		j, err = s.Get(ctx, id)
		if err == nil && (j.State == StateFinished || j.State == StateFailed) {
			t.Logf("%s at %s, content %q", j.State, time.Now().Format("15:04:05.000"), j.ContentPath)
			if j.ContentPath != "" || j.State == StateFailed {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("not done: %+v %v", j, err)
		}
		time.Sleep(time.Second)
	}
	h := raw("history", url.Values{"category": {"tv"}, "limit": {"5"}})
	if i := strings.Index(h, `"storage":`); i >= 0 {
		t.Logf("SABnzbd storage: %s", h[i:i+strings.Index(h[i:], ",")])
	}
	t.Logf("job %s: state %s, content %s", j.Name, j.State, j.ContentPath)
	if j.State != StateFinished {
		t.Fatalf("failed: %s", j.Detail)
	}
	pm := []domain.PathMap{{Theirs: "/data/downloads", Ours: filepath.ToSlash(disk)}}
	save, seen := readFinished(j, pm, disk)
	var cands []importer.Candidate
	for _, f := range seen {
		cands = append(cands, importer.Candidate{Path: filepath.Join(mapLocal(save, pm), filepath.FromSlash(f.Name)), Size: f.Size})
	}
	var eps []importer.Episode
	for e := 1; e <= 10; e++ {
		eps = append(eps, importer.Episode{Ref: mediafiles.EpisodeRef{ItemID: int64(e), Season: 2, Episode: e}, Name: fmt.Sprintf("Ep %d", e), SlotID: int64(100 + e), Wanted: true})
	}
	ns := mediafiles.NamingDefaults()
	steps := importer.PlanEpisodes(cands, importer.ShowTarget{LibraryPath: t.TempDir(), FolderName: "Lab Pack", Show: mediafiles.TitleRef{Name: "Lab Pack"},
		Order: mediafiles.OrderAired, Naming: &ns, Episodes: eps, ReleaseName: pack})
	got := map[int64]bool{}
	for _, st := range steps {
		if strings.EqualFold(filepath.Ext(st.SourceFile), ".mkv") {
			t.Logf("%s -> %v %s", filepath.Base(st.SourceFile), st.TargetSlotIDs, st.Method)
		}
		for _, sid := range st.TargetSlotIDs {
			got[sid] = true
		}
	}
	t.Logf("read %d file(s), matched %d of 10 episodes", len(seen), len(got))
	if len(got) != 10 {
		t.Fatalf("matched %d of 10 episodes", len(got))
	}
}
