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

// TestSingleTitlePass: the matches the earlier passes kept on one title in
// common are weighed again; one whose title is not rare becomes a miss and
// loses that source's pictures (backed up first, then refetched); a rare
// one stays; a match changed since is left alone; it runs once.
func TestSingleTitlePass(t *testing.T) {
	const (
		roxana = "c1000000-0000-4000-8000-000000000001" // Apple, "Fall": dropped
		tweedy = "c2000000-0000-4000-8000-000000000002" // Apple, "Sukierae": kept
		lyly   = "c3000000-0000-4000-8000-000000000003" // Apple via the shared pass: dropped
		since  = "c4000000-0000-4000-8000-000000000004" // relinked since: untouched
		many   = "c5000000-0000-4000-8000-000000000005" // two titles: not asked
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	imgs.KeepID(imgstore.KindArtist, roxana, srctag.Tag{Source: "applemusic"}, "us", "1")
	imgs.KeepID(imgstore.KindArtist, tweedy, srctag.Tag{Source: "applemusic"}, "us", "2")
	imgs.KeepID(imgstore.KindArtist, lyly, srctag.Tag{Source: "applemusic"}, "us", "3")
	imgs.KeepID(imgstore.KindArtist, since, srctag.Tag{Source: "applemusic"}, "us", "999")
	imgs.KeepID(imgstore.KindArtist, many, srctag.Tag{Source: "applemusic"}, "us", "5")
	dir := t.TempDir()
	write := func(name string, v any) {
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("apple-homonym-reeval-20261029-224641.json", []homonymResult{
		{MBID: roxana, Name: "Roxana", Store: "us", AppleID: "1", Decision: "kept", Have: 1},
		{MBID: tweedy, Name: "Tweedy", Store: "us", AppleID: "2", Decision: "restore", Have: 1},
		{MBID: since, Name: "Low", Store: "us", AppleID: "4", Decision: "kept", Have: 1},
		{MBID: many, Name: "Many", Store: "us", AppleID: "5", Decision: "kept", Have: 2},
	})
	// An older re-check file is ignored.
	write("apple-homonym-reeval-20261029-213946.json", []homonymResult{{MBID: many, Name: "Many", Store: "us", AppleID: "5", Decision: "restore", Have: 1}})
	write("shared-ids-20261029-225118.json", []sharedResult{
		{MBID: lyly, Name: "LYLY", Source: "applemusic", Store: "us", ID: "3", Decision: "keep", Have: 1, SharedWith: []string{since}},
	})
	var asked, refetched []string
	p := &singleTitlePass{imgs: imgs, dir: dir, now: time.Now, logf: t.Logf,
		rivals: func(context.Context, string, string) []string { return nil },
		evidence: func(_ context.Context, source, store, id, mbid string, rivals []string) (imgstore.AppleEvidence, error) {
			asked = append(asked, mbid)
			if mbid == lyly && (len(rivals) != 1 || rivals[0] != since) {
				t.Fatalf("lyly rivals %v", rivals)
			}
			return imgstore.AppleEvidence{OK: mbid == tweedy, Have: 1, Single: "x", SingleHolders: 7}, nil
		},
		refetch: func(_ context.Context, mbid string) { refetched = append(refetched, mbid) }}
	ctx := context.Background()
	kept, dropped, err := p.run(ctx)
	if err != nil || kept != 1 || dropped != 2 {
		t.Fatalf("kept %d dropped %d %v", kept, dropped, err)
	}
	if len(asked) != 3 {
		t.Fatalf("asked %v", asked)
	}
	for id, want := range map[string]string{roxana: "", tweedy: "2", lyly: "", since: "999", many: "5"} {
		if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, id, "applemusic"); ext != want {
			t.Errorf("%s: %q, want %q", id, ext, want)
		}
	}
	if len(refetched) != 2 {
		t.Fatalf("refetched %v", refetched)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "single-title-*.json"))
	if len(files) != 1 {
		t.Fatalf("result files %v", files)
	}
	if k, d, _ := p.run(ctx); k+d != 0 {
		t.Fatal("pass ran twice")
	}
}
