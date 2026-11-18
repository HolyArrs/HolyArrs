package updates_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates/signtest"
)

// release serves one program and its signature.
type release struct {
	srv  *httptest.Server
	body []byte
	sig  []byte
	hits atomic.Int32
}

func newRelease(t *testing.T, k signtest.Key, body string) *release {
	r := &release{body: []byte(body), sig: k.Sign([]byte(body), "holyarrs")}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.hits.Add(1)
		switch q.URL.Path {
		case "/holyarrs":
			_, _ = w.Write(r.body)
		case "/holyarrs.minisig":
			_, _ = w.Write(r.sig)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *release) channel(version string) updates.Channel {
	sum := sha256.Sum256(r.body)
	return updates.Channel{Version: version, Files: map[string]updates.File{
		updates.Platform(): {URL: r.srv.URL + "/holyarrs", Size: int64(len(r.body)), SHA256: hex.EncodeToString(sum[:])},
	}}
}

func (v *volume) installer(restarts *int) *updates.Installer {
	return &updates.Installer{
		ConfigDir: v.cfg, Version: "1.1.4", Keys: []updates.PublicKey{v.key.Pub}, Client: http.DefaultClient,
		Check:   func(context.Context, string) error { return nil },
		Restart: func() { *restarts++ },
	}
}

var emptyList = &updates.List{}

func TestInstallDownloadsChecksAndSwitches(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program 1.2.0")
	n := 0
	in := v.installer(&n)
	if err := in.Prepare(context.Background(), emptyList, nil, nil, rel.channel("1.2.0")); err != nil {
		t.Fatal(err)
	}
	if in.Progress().Phase != updates.PhaseReady {
		t.Fatalf("phase %+v", in.Progress())
	}
	if err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("restarts %d", n)
	}
	cur, ok := v.launcher().Pick()
	if !ok || cur.Version != "1.2.0" {
		t.Fatalf("launcher picked %v %v", cur, ok)
	}
}

func TestInstallRefusesBadDownloads(t *testing.T) {
	for name, spoil := range map[string]func(r *release, c *updates.Channel){
		"changed byte": func(r *release, _ *updates.Channel) { r.body[0] ^= 1 },
		"unknown key":  func(r *release, _ *updates.Channel) { r.sig = signtest.New(t).Sign(r.body, "x") },
		"wrong size": func(_ *release, c *updates.Channel) {
			f := c.Files[updates.Platform()]
			f.Size--
			c.Files[updates.Platform()] = f
		},
		"no signature":  func(r *release, _ *updates.Channel) { r.sig = nil },
		"no file here":  func(_ *release, c *updates.Channel) { c.Files = nil },
		"image only":    func(_ *release, c *updates.Channel) { c.ImageRequired = true },
		"too old":       func(_ *release, c *updates.Channel) { c.MinFrom = "1.1.6" },
		"new data form": func(_ *release, c *updates.Channel) { c.DataCompat = 2 },
		"not newer":     func(_ *release, c *updates.Channel) { c.Version = "1.1.4" },
	} {
		t.Run(name, func(t *testing.T) {
			v := newVolume(t)
			rel := newRelease(t, v.key, "program 1.2.0")
			c := rel.channel("1.2.0")
			spoil(rel, &c)
			n := 0
			in := v.installer(&n)
			if err := in.Prepare(context.Background(), emptyList, nil, nil, c); err == nil {
				t.Fatal("prepared a bad release")
			}
			if v.exists(c.Version) {
				t.Fatal("bad download kept")
			}
			if err := in.Install(context.Background()); !errors.Is(err, updates.ErrNotReady) || n != 0 {
				t.Fatalf("installed anyway: %v", err)
			}
		})
	}
}

func TestInstallRevokedAndSkippedNotOffered(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program")
	n := 0
	if err := v.installer(&n).Prepare(context.Background(), &updates.List{Revoked: []string{"1.2.0"}}, nil, nil, rel.channel("1.2.0")); err == nil {
		t.Fatal("revoked version prepared")
	}
	v.set("skip.json", []string{"1.2.0"})
	if err := v.installer(&n).Prepare(context.Background(), emptyList, nil, nil, rel.channel("1.2.0")); err == nil || !strings.Contains(err.Error(), "skipped") {
		t.Fatalf("skipped version: %v", err)
	}
	if rel.hits.Load() != 0 {
		t.Fatal("downloaded a version it may not install")
	}
}

func TestInstallFailedDryRunDeletes(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program")
	n := 0
	in := v.installer(&n)
	in.Check = func(context.Context, string) error { return errors.New("permission denied") }
	if err := in.Prepare(context.Background(), emptyList, nil, nil, rel.channel("1.2.0")); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("dry run failure not reported: %v", err)
	}
	if v.exists("1.2.0") {
		t.Fatal("download kept after a failed dry run")
	}
}

func TestInstallWaitsForIdleAndGivesUp(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program")
	n := 0
	in := v.installer(&n)
	in.IdleWait = 30 * time.Millisecond
	in.Busy = func() string { return "2 imports" }
	if err := in.Prepare(context.Background(), emptyList, nil, nil, rel.channel("1.2.0")); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "2 imports") {
		t.Fatalf("did not give up: %v", err)
	}
	if n != 0 || v.exists("current.json") {
		t.Fatal("switched while busy")
	}
}

func TestInstallMigratingCopiesDatabaseFirst(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program")
	n := 0
	in := v.installer(&n)
	in.Snapshot = func(dir string) error { return os.WriteFile(filepath.Join(dir, "holyarrs.db"), []byte("copy"), 0o600) }
	c := rel.channel("1.2.0")
	c.Migrates = true
	if err := in.Prepare(context.Background(), emptyList, nil, nil, c); err != nil {
		t.Fatal(err)
	}
	if err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !v.exists(filepath.Join("dbcopy", "holyarrs.db")) {
		t.Fatal("no database copy")
	}
}

func TestInstallFailedCopyChangesNothing(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program")
	n := 0
	in := v.installer(&n)
	in.Snapshot = func(string) error { return errors.New("disk full") }
	c := rel.channel("1.2.0")
	c.Migrates = true
	_ = in.Prepare(context.Background(), emptyList, nil, nil, c)
	if err := in.Install(context.Background()); err == nil || n != 0 || v.exists("current.json") {
		t.Fatalf("switched without a copy: %v", err)
	}
}

func TestGoBackSkipsRunningVersion(t *testing.T) {
	t.Setenv("HOLYARRS_FROM_VOLUME", "1")
	v := newVolume(t)
	v.set("previous.json", v.install("1.2.0", "previous"))
	v.set("current.json", v.install("1.3.0", "current"))
	n := 0
	in := v.installer(&n)
	in.Version = "1.3.0"
	if in.Previous("1.1.4") != "1.2.0" {
		t.Fatalf("previous %q", in.Previous("1.1.4"))
	}
	if err := in.GoBack(); err != nil || n != 1 {
		t.Fatalf("go back: %v, restarts %d", err, n)
	}
	t.Setenv("HOLYARRS_FROM_VOLUME", "")
	cur, ok := v.launcher().Pick()
	if !ok || cur.Version != "1.2.0" {
		t.Fatalf("after go back picked %v %v", cur, ok)
	}
}

func TestGoBackRefusedOnImageProgram(t *testing.T) {
	v := newVolume(t)
	n := 0
	if err := v.installer(&n).GoBack(); err == nil || n != 0 {
		t.Fatal("go back on the image program")
	}
}

func TestDownloadAllowedHosts(t *testing.T) {
	for link, want := range map[string]bool{
		"https://github.com/HolyArrs/HolyArrs/releases/download/1.2.0/holyarrs-linux-amd64": true,
		"https://objects.githubusercontent.com/x":                                           true,
		"http://github.com/x":               false,
		"https://github.com:8443/x":         false,
		"https://user@github.com/x":         false,
		"https://evil.example/x":            false,
		"https://github.com.evil.example/x": false,
	} {
		if updates.DownloadAllowed(link) != want {
			t.Errorf("%s: want %v", link, want)
		}
	}
}

func TestPreviousIsImageVersionFromLauncher(t *testing.T) {
	t.Setenv("HOLYARRS_FROM_VOLUME", "1")
	t.Setenv("HOLYARRS_IMAGE_VERSION", "1.1.4")
	v := newVolume(t)
	n := 0
	in := v.installer(&n)
	in.Version = "1.2.0"
	if got := in.Previous("1.2.0"); got != "1.1.4" {
		t.Fatalf("previous %q, want the image's 1.1.4", got)
	}
}
