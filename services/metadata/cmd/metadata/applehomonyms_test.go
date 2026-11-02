package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// TestHomonymRestore: the one-off re-check of the matches the first
// homonym pass dropped restores those the album evidence carries (match
// kept again, pictures, similar list and top songs marked for a refetch),
// keeps the rest a miss, leaves a match made since alone, writes every
// decision to a file and runs once.
func TestHomonymRestore(t *testing.T) {
	const (
		good  = "a74b1b7f-71a5-4011-9441-d0b5e4122711"
		bad   = "b84b1b7f-71a5-4011-9441-d0b5e4122712"
		since = "c94b1b7f-71a5-4011-9441-d0b5e4122713"
		redo  = "e14b1b7f-71a5-4011-9441-d0b5e4122714"
		stale = "f24b1b7f-71a5-4011-9441-d0b5e4122715"
		lone  = "f34b1b7f-71a5-4011-9441-d0b5e4122716"
		rival = "d04b1b7f-71a5-4011-9441-d0b5e4122719"
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	for _, id := range []string{good, bad} {
		imgs.KeepID(imgstore.KindArtist, id, srctag.Tag{Source: "applemusic"}, "", "")
	}
	imgs.KeepID(imgstore.KindArtist, since, srctag.Tag{Source: "applemusic"}, "us", "999")
	// An earlier re-check restored this one; the rule now refuses it.
	imgs.KeepID(imgstore.KindArtist, redo, srctag.Tag{Source: "applemusic"}, "us", "444")
	// Misses the old rule kept at lookup time: one for a name others carry
	// (cleared), one for a unique name (a real miss, left alone).
	window := time.Now()
	imgs.KeepID(imgstore.KindArtist, stale, srctag.Tag{Source: "applemusic"}, "", "")
	imgs.KeepID(imgstore.KindArtist, lone, srctag.Tag{Source: "applemusic"}, "", "")
	dir := t.TempDir()
	drops := []homonymDrop{{MBID: good, Name: "Silica Gel", Store: "us", AppleID: "111"},
		{MBID: bad, Name: "Utopia", Store: "us", AppleID: "222"}, {MBID: since, Name: "Low", Store: "us", AppleID: "333"},
		{MBID: redo, Name: "Hana", Store: "us", AppleID: "444"}}
	b, _ := json.Marshal(drops)
	if err := os.WriteFile(filepath.Join(dir, "apple-homonyms-20261029-205045.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	var refetched, asked, undone []string
	p := &homonymRestore{imgs: imgs, dir: dir, now: time.Now, logf: t.Logf,
		staleFrom: window.Add(-time.Minute), staleTo: window.Add(time.Minute),
		name: func(_ context.Context, id string) string {
			if id == lone {
				return "Unique"
			}
			return "Kashmir"
		},
		rivals: func(_ context.Context, name, except string) []string {
			if name == "Unique" {
				return nil
			}
			return []string{rival}
		},
		titles: func(_ context.Context, id string) []string { return []string{"T-" + id[:1]} },
		evidence: func(_ context.Context, store, appleID, mbid string, rivals []string) (imgstore.AppleEvidence, error) {
			asked = append(asked, mbid)
			if len(rivals) != 1 || rivals[0] != rival || store != "us" {
				t.Fatalf("asked with %q %v", store, rivals)
			}
			return imgstore.AppleEvidence{OK: appleID == "111", Have: 4, Apple: []string{"Machine Boy"}}, nil
		},
		refetch: func(_ context.Context, mbid string) { refetched = append(refetched, mbid) },
		undo: func(_ context.Context, mbid string) {
			undone = append(undone, mbid)
			imgs.KeepID(imgstore.KindArtist, mbid, srctag.Tag{Source: "applemusic"}, "", "")
		}}
	ctx := context.Background()
	restored, rejected, err := p.run(ctx)
	if err != nil || restored != 1 || rejected != 2 {
		t.Fatalf("restored %d rejected %d %v", restored, rejected, err)
	}
	if st, ext, _, _ := imgs.KeptID(imgstore.KindArtist, good, "applemusic"); st != "us" || ext != "111" {
		t.Fatalf("not restored %q %q", st, ext)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, bad, "applemusic"); ext != "" {
		t.Fatalf("rejected one restored %q", ext)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, since, "applemusic"); ext != "999" {
		t.Fatalf("a later match was touched %q", ext)
	}
	if len(undone) != 1 || undone[0] != redo {
		t.Fatalf("undone %v", undone)
	}
	if _, _, _, ok := imgs.KeptID(imgstore.KindArtist, stale, "applemusic"); ok {
		t.Fatal("stale miss not cleared")
	}
	if _, _, _, ok := imgs.KeptID(imgstore.KindArtist, lone, "applemusic"); !ok {
		t.Fatal("a real miss was cleared")
	}
	if len(refetched) != 2 || refetched[0] != good || refetched[1] != stale || len(asked) != 3 {
		t.Fatalf("refetched %v asked %v", refetched, asked)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "apple-homonym-reeval-*.json"))
	if len(files) != 1 {
		t.Fatalf("result files %v", files)
	}
	out, _ := os.ReadFile(files[0])
	var res []homonymResult
	if json.Unmarshal(out, &res) != nil || len(res) != 5 || res[4].Decision != "stale-cleared" || res[0].Decision != "restore" || res[1].Decision != "reject" || res[2].Decision != "matched-since" || res[3].Decision != "undo" || len(res[0].MBTitles) == 0 {
		t.Fatalf("results %s", out)
	}
	if r, j, _ := p.run(ctx); r+j != 0 || len(asked) != 3 {
		t.Fatal("pass ran twice")
	}
}
