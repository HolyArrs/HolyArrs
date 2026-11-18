package updates_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates/signtest"
)

func TestLaunchOffEnvIgnoresCaseAndSpaces(t *testing.T) {
	for _, val := range []string{"Off", "OFF", " off "} {
		v := newVolume(t)
		v.set("current.json", v.install("1.2.0", "new program"))
		v.env["HOLYARRS_UPDATES"] = val
		if _, ok := v.launcher().Pick(); ok {
			t.Fatalf("%q still picked the download", val)
		}
	}
}

func TestRestoreCopyKeepsNewerDataAside(t *testing.T) {
	v := newVolume(t)
	writeFile(t, filepath.Join(v.cfg, database.MainFile), "newer")
	writeFile(t, filepath.Join(v.cfg, database.MainFile+"-wal"), "newer wal")
	writeFile(t, v.app(filepath.Join("dbcopy", database.MainFile)), "before")
	if err := updates.RestoreCopy(v.cfg); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(v.cfg, database.MainFile))
	if string(b) != "before" {
		t.Fatalf("database is %q", b)
	}
	kept, _ := filepath.Glob(filepath.Join(v.cfg, "backups", "before-restore-*", database.MainFile+"*"))
	if len(kept) != 2 {
		t.Fatalf("newer data not kept aside: %v", kept)
	}
	got, _ := os.ReadFile(kept[0])
	if string(got) != "newer" {
		t.Fatalf("kept %q", got)
	}
}

func TestInstallTwiceKeepsPrevious(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program 1.2.0")
	n := 0
	in := v.installer(&n)
	v.set("current.json", updates.Installed{Version: "1.1.4"})
	if err := in.Prepare(context.Background(), emptyList, nil, nil, rel.channel("1.2.0")); err != nil {
		t.Fatal(err)
	}
	// A double click: the second call comes while the first restarts.
	for range 2 {
		_ = in.Install(context.Background())
	}
	if n != 1 {
		t.Fatalf("restarts %d", n)
	}
	if !v.exists("previous.json") {
		t.Fatal("second install removed the go-back target")
	}
}

func TestPrepareTwiceAtOnceDownloadsOnce(t *testing.T) {
	v := newVolume(t)
	rel := newRelease(t, v.key, "program 1.2.0")
	n := 0
	in := v.installer(&n)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = in.Prepare(context.Background(), emptyList, nil, nil, rel.channel("1.2.0"))
		}()
	}
	wg.Wait()
	if _, ok := in.Staged(); !ok {
		t.Fatal("nothing staged")
	}
	if h := rel.hits.Load(); h > 2 {
		t.Fatalf("downloaded %d times", h/2)
	}
	if err := in.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cur, ok := v.launcher().Pick(); !ok || cur.Version != "1.2.0" {
		t.Fatalf("staged file missing: %v %v", cur, ok)
	}
}

func TestParseDropsUnsafeVersions(t *testing.T) {
	k := signtest.New(t)
	for _, bad := range []string{"9.0.0-x/../../../tmp/y", "1.2.0/x", "..", "1.2", "1.2.0-a b"} {
		data := []byte(strings.Replace(string(listJSON("1", "2026-10-23T11:00:00Z", "")), `"v1.2.0"`, `"`+bad+`"`, 1))
		l, err := updates.Parse(data, k.Sign(data, "t"), []updates.PublicKey{k.Pub}, now, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := l.Offer("stable", 0); ok {
			t.Errorf("%q offered", bad)
		}
	}
	data := listJSON("1", "2026-10-23T11:00:00Z", "")
	l, _ := updates.Parse(data, k.Sign(data, "t"), []updates.PublicKey{k.Pub}, now, 0)
	if _, ok := l.Offer("stable", 0); !ok {
		t.Error("good version dropped")
	}
}
