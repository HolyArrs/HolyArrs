package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// TestSharedIDPass: a kept id that MusicBrainz links differently is
// replaced by the link; in a group of artists sharing one id the linked
// one keeps it, an unlinked one keeps it only on album evidence over the
// others, the rest become misses; every change is backed up first and
// refetched; the pass runs once.
func TestSharedIDPass(t *testing.T) {
	const (
		yoru   = "a1000000-0000-4000-8000-000000000001" // Deezer: kept 92503082, linked 67898492
		ms     = "a2000000-0000-4000-8000-000000000002" // Apple 21434, linked
		lauryn = "a3000000-0000-4000-8000-000000000003" // Apple 21434, not linked: dropped
		lucyA  = "a4000000-0000-4000-8000-000000000004" // Spotify sp1: evidence holds
		lucyB  = "a5000000-0000-4000-8000-000000000005" // Spotify sp1: no evidence, dropped
		lone   = "a6000000-0000-4000-8000-000000000006" // Spotify sp2 alone: untouched
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	imgs.KeepID(imgstore.KindArtist, yoru, srctag.Tag{Source: "deezer"}, imgstore.MatchSearch, "92503082")
	imgs.KeepID(imgstore.KindArtist, ms, srctag.Tag{Source: "applemusic"}, "us", "21434")
	imgs.KeepID(imgstore.KindArtist, lauryn, srctag.Tag{Source: "applemusic"}, "us", "21434")
	imgs.KeepID(imgstore.KindArtist, lucyA, srctag.Tag{Source: "spotify"}, "search", "sp1")
	imgs.KeepID(imgstore.KindArtist, lucyB, srctag.Tag{Source: "spotify"}, "search", "sp1")
	imgs.KeepID(imgstore.KindArtist, lone, srctag.Tag{Source: "spotify"}, "search", "sp2")
	briefs := map[string]mbread.Brief{
		yoru:   {Name: "ヨルシカ", Links: []string{"https://www.deezer.com/artist/67898492"}},
		ms:     {Name: "Ms. Lauryn Hill", Links: []string{"https://music.apple.com/us/artist/ms-lauryn-hill/21434"}},
		lauryn: {Name: "Lauryn Hill"},
		lucyA:  {Name: "Lucy"},
		lucyB:  {Name: "Lucy"},
		lone:   {Name: "Unique"},
	}
	var asked, refetched []string
	dir := t.TempDir()
	p := &sharedPass{imgs: imgs, dir: dir, now: time.Now, logf: t.Logf,
		brief:  func(_ context.Context, id string) (mbread.Brief, bool) { b, ok := briefs[id]; return b, ok },
		rivals: func(context.Context, string, string) []string { return nil },
		titles: func(context.Context, string) []string { return []string{"x"} },
		evidence: func(_ context.Context, source, store, id, mbid string, rivals []string) (imgstore.AppleEvidence, error) {
			asked = append(asked, source+"/"+mbid)
			if len(rivals) != 1 {
				t.Fatalf("%s rivals %v", mbid, rivals)
			}
			return imgstore.AppleEvidence{OK: mbid == lucyA, Have: 3}, nil
		},
		similar: func(mbid, source string) []byte { return nil },
		refetch: func(_ context.Context, mbid string) { refetched = append(refetched, mbid) }}
	ctx := context.Background()
	n, err := p.run(ctx)
	if err != nil || n != 3 {
		t.Fatalf("changed %d %v", n, err)
	}
	want := map[string]struct{ source, ext string }{
		yoru: {"deezer", "67898492"}, ms: {"applemusic", "21434"}, lauryn: {"applemusic", ""},
		lucyA: {"spotify", "sp1"}, lucyB: {"spotify", ""}, lone: {"spotify", "sp2"},
	}
	for id, w := range want {
		if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, id, w.source); ext != w.ext {
			t.Errorf("%s %s: %q, want %q", id, w.source, ext, w.ext)
		}
	}
	if len(refetched) != 3 {
		t.Fatalf("refetched %v", refetched)
	}
	// Ms. Lauryn Hill is linked: never asked. Lauryn Hill and both Lucys are.
	if len(asked) != 3 {
		t.Fatalf("asked %v", asked)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "shared-ids-*.json"))
	if len(files) != 1 {
		t.Fatalf("backup files %v", files)
	}
	b, _ := os.ReadFile(files[0])
	var back []sharedResult
	if json.Unmarshal(b, &back) != nil || len(back) != 5 {
		t.Fatalf("backup %s", b)
	}
	if n, _ := p.run(ctx); n != 0 {
		t.Fatal("pass ran twice")
	}
}

// A source that cannot answer (paused) leaves its group untouched and the
// pass open; the other sources' changes are made, and a later run finishes.
func TestSharedIDPassSkipsPausedSource(t *testing.T) {
	const (
		a = "b1000000-0000-4000-8000-000000000001"
		b = "b2000000-0000-4000-8000-000000000002"
		c = "b3000000-0000-4000-8000-000000000003"
		d = "b4000000-0000-4000-8000-000000000004"
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	imgs.KeepID(imgstore.KindArtist, a, srctag.Tag{Source: "deezer"}, "search", "1")
	imgs.KeepID(imgstore.KindArtist, b, srctag.Tag{Source: "deezer"}, "search", "1")
	imgs.KeepID(imgstore.KindArtist, c, srctag.Tag{Source: "spotify"}, "search", "s")
	imgs.KeepID(imgstore.KindArtist, d, srctag.Tag{Source: "spotify"}, "search", "s")
	paused := true
	p := &sharedPass{imgs: imgs, dir: t.TempDir(), now: time.Now, logf: t.Logf,
		brief:  func(context.Context, string) (mbread.Brief, bool) { return mbread.Brief{Name: "Lucy"}, true },
		rivals: func(context.Context, string, string) []string { return nil },
		titles: func(context.Context, string) []string { return nil },
		evidence: func(_ context.Context, source, _, _, mbid string, _ []string) (imgstore.AppleEvidence, error) {
			if source == "spotify" && paused {
				return imgstore.AppleEvidence{}, errors.New("imgstore: source paused")
			}
			return imgstore.AppleEvidence{OK: mbid == a || mbid == c}, nil
		},
		refetch: func(context.Context, string) {}}
	ctx := context.Background()
	n, err := p.run(ctx)
	if err != nil || n != 1 {
		t.Fatalf("first run changed %d %v", n, err)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, b, "deezer"); ext != "" {
		t.Fatal("deezer group not checked")
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, d, "spotify"); ext != "s" {
		t.Fatal("a paused source's group was changed")
	}
	if p.imgs.KV(sharedIDsFlag) != "" {
		t.Fatal("pass closed with a group left")
	}
	paused = false
	if n, err := p.run(ctx); err != nil || n != 1 {
		t.Fatalf("second run changed %d %v", n, err)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, d, "spotify"); ext != "" {
		t.Fatal("spotify group not checked on the second run")
	}
	if p.imgs.KV(sharedIDsFlag) == "" {
		t.Fatal("pass left open")
	}
}
