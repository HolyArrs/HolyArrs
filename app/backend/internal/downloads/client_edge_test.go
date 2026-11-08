package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestMetadataRefusedEvenWithLocalNetwork(t *testing.T) {
	for _, u := range []string{"http://169.254.169.254/latest", "http://[fe80::1]:80/", "http://metadata.google.internal/", "http://metadata/"} {
		c := Client(u, true)
		c.Timeout = 3 * time.Second
		resp, err := c.Get(u)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("%s answered", u)
		}
		if !errors.Is(err, outbound.ErrAddressRefused) {
			t.Fatalf("%s: %v", u, err)
		}
	}
}

func TestSabnzbdRefusalSaysNoBody(t *testing.T) {
	f := &fakeSab{refuse: "secret-looking line\nsecond line"}
	_, err := NewSabnzbd(startSab(t, f), sabKey, nil).Health(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret-looking") || strings.Contains(err.Error(), "second") {
		t.Fatalf("refusal: %v", err)
	}
}

func TestRedirectKeepsTheScheme(t *testing.T) {
	c := Client("https://dl.example:8443", false)
	from, _ := url.Parse("https://dl.example:8443/api")
	to, _ := url.Parse("http://dl.example:8443/api")
	if err := c.CheckRedirect(&http.Request{URL: to}, []*http.Request{{URL: from}}); err == nil {
		t.Fatal("https to http allowed")
	}
	same, _ := url.Parse("https://dl.example:8443/api/v2")
	if err := c.CheckRedirect(&http.Request{URL: same}, []*http.Request{{URL: from}}); err != nil {
		t.Fatalf("same scheme refused: %v", err)
	}
}

func TestDangerousNamesHidden(t *testing.T) {
	for _, n := range []string{"a/movie.exe.", "movie.exe ", "x.hta", "x.wsf", "x.wsh", "x.vbe", "x.jse", "x.msc", "x.cpl", "x.scf", "x.reg", "x.dll",
		"x.sh", "x.AppImage", "movie‮vkm.exe", "movie‭.mkv", "a⁦b.mkv", "a⁩b.mkv"} {
		if len(dangerous([]File{{Name: n}})) != 1 {
			t.Fatalf("%q not flagged", n)
		}
	}
	if len(dangerous([]File{{Name: "Dune/movie.mkv"}, {Name: "Dune/movie.en.srt"}})) != 0 {
		t.Fatal("plain files flagged")
	}
}

func TestReadFinishedOnlyUnderARoot(t *testing.T) {
	dir := t.TempDir()
	job := filepath.Join(dir, "Job")
	deep := filepath.Join(job, "a", "b", "c", "d", "e", "f", "g", "h", "i")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(job, "movie.mkv"), make([]byte, 10), 0o644)
	_ = os.WriteFile(filepath.Join(deep, "far.mkv"), make([]byte, 1), 0o644)
	if _, files := readFinished(Job{ContentPath: job}, nil, "/somewhere/else"); len(files) != 0 {
		t.Fatalf("walked outside every root: %+v", files)
	}
	if _, files := readFinished(Job{ContentPath: filepath.Join(dir, "x") + string(filepath.Separator) + ".." + string(filepath.Separator) + "Job"}, nil, filepath.Join(dir, "x")); len(files) != 0 {
		t.Fatalf("walked through ..: %+v", files)
	}
	_, files := readFinished(Job{ContentPath: job}, nil, dir)
	if len(files) != 1 || files[0].Name != "Job/movie.mkv" {
		t.Fatalf("under the save root: %+v", files)
	}
}

func TestRefusedReleaseDoesNotRestTheDownloader(t *testing.T) {
	w, m := setup(t)
	w.fake.addFails = true
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil {
		t.Fatal("refused add succeeded")
	}
	if d := w.downloader(t, 1); d.BackoffUntil != nil || d.State != "ok" || d.FailCount != 0 {
		t.Fatalf("downloader put to rest: %+v", d)
	}
}

func TestSpaceHonoursTheContext(t *testing.T) {
	_, m := setup(t)
	done, err := m.space(context.Background(), "http://h:1")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := m.space(ctx, "http://h:1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("waited past the context")
	}
}
