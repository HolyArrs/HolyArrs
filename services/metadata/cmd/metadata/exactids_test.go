package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// TestExactIDsPass: every unclassified match goes through the order: its
// MusicBrainz link, a barcode, Wikidata, then the strict name rule. Same id:
// only how it was found is kept; another id: the match changes; a name
// match that fails the rule becomes a miss. Changes are backed up first
// and refetched; a paused source is waited out; the pass ends when every
// row is classified.
func TestExactIDsPass(t *testing.T) {
	const (
		linked  = "e1000000-0000-4000-8000-000000000001" // Apple link = kept
		same    = "e2000000-0000-4000-8000-000000000002" // Deezer barcode = kept
		moved   = "e3000000-0000-4000-8000-000000000003" // Spotify barcode != kept: changed
		wd      = "e4000000-0000-4000-8000-000000000004" // Apple Wikidata = kept
		unique  = "e5000000-0000-4000-8000-000000000005" // Deezer name, no homonym: kept
		weak    = "e6000000-0000-4000-8000-000000000006" // Apple name, homonym, no evidence: dropped
		strong  = "e7000000-0000-4000-8000-000000000007" // Spotify name, homonym, evidence: kept
		relinkd = "e8000000-0000-4000-8000-000000000008" // Deezer link != kept: changed to the link
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	keep := func(id, source, store, ext string) {
		imgs.KeepID(imgstore.KindArtist, id, srctag.Tag{Source: source}, store, ext)
	}
	keep(linked, "applemusic", "us", "100")
	keep(same, "deezer", "search", "200")
	keep(moved, "spotify", "search", "old")
	keep(wd, "applemusic", "us", "400")
	keep(unique, "deezer", "search", "500")
	keep(weak, "applemusic", "us", "600")
	keep(strong, "spotify", "search", "700")
	keep(relinkd, "deezer", "search", "800")
	briefs := map[string]mbread.Brief{
		linked:  {Name: "A", Links: []string{"https://music.apple.com/us/artist/a/100"}},
		same:    {Name: "B"},
		moved:   {Name: "C"},
		wd:      {Name: "D", Links: []string{"https://www.wikidata.org/wiki/Q4"}},
		unique:  {Name: "Unique"},
		weak:    {Name: "Lucy"},
		strong:  {Name: "Lucy"},
		relinkd: {Name: "E", Links: []string{"https://www.deezer.com/artist/801"}},
	}
	paused := 1
	var prefetched []string
	var refetched []string
	dir := t.TempDir()
	p := &exactIDsPass{imgs: imgs, dir: dir, now: time.Now, logf: t.Logf, wait: time.Millisecond,
		brief: func(_ context.Context, id string) (mbread.Brief, bool) { b, ok := briefs[id]; return b, ok },
		rivals: func(_ context.Context, name, _ string) []string {
			if name == "Lucy" {
				return []string{"rival"}
			}
			return nil
		},
		prefetch: func(_ context.Context, qids []string) { prefetched = append(prefetched, qids...) },
		exact: func(_ context.Context, source, mbid, _, qid, _ string) (string, string, string, error) {
			switch mbid {
			case same:
				return "200", "", imgstore.MatchBarcode, nil
			case moved:
				if paused > 0 {
					paused--
					return "", "", "", errors.New("spotify: imgstore: source paused")
				}
				return "new", "", imgstore.MatchBarcode, nil
			case wd:
				if qid != "Q4" {
					t.Fatalf("qid %q", qid)
				}
				return "400", "", imgstore.MatchWikidata, nil
			}
			return "", "", "", nil
		},
		evidence: func(_ context.Context, _, _, _, mbid string, _ []string) (imgstore.AppleEvidence, error) {
			return imgstore.AppleEvidence{OK: mbid == strong}, nil
		},
		refetch: func(_ context.Context, mbid string) { refetched = append(refetched, mbid) }}
	ctx := context.Background()
	counts, err := p.run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ source, ext, via string }{
		linked:  {"applemusic", "100", imgstore.ViaLink},
		same:    {"deezer", "200", imgstore.ViaBarcode},
		moved:   {"spotify", "new", imgstore.ViaBarcode},
		wd:      {"applemusic", "400", imgstore.ViaWikidata},
		unique:  {"deezer", "500", imgstore.ViaName},
		weak:    {"applemusic", "", viaChecked},
		strong:  {"spotify", "700", imgstore.ViaName},
		relinkd: {"deezer", "801", imgstore.ViaLink},
	}
	for id, w := range want {
		_, ext, _, _ := imgs.KeptID(imgstore.KindArtist, id, w.source)
		if ext != w.ext || imgs.FoundBy(imgstore.KindArtist, id, w.source) != w.via {
			t.Errorf("%s: %q via %q, want %q via %q", id, ext, imgs.FoundBy(imgstore.KindArtist, id, w.source), w.ext, w.via)
		}
	}
	if counts["spotify barcode changed"] != 1 || counts["applemusic name dropped"] != 1 || counts["deezer mb-link changed"] != 1 || counts["deezer barcode same"] != 1 {
		t.Fatalf("counts %v", counts)
	}
	if len(refetched) != 3 || len(prefetched) != 1 || prefetched[0] != "Q4" {
		t.Fatalf("refetched %v prefetched %v", refetched, prefetched)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "exact-ids-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	b, _ := os.ReadFile(files[0])
	if strings.Count(string(b), "\n") != 9 {
		t.Fatalf("result lines:\n%s", b)
	}
	if c, _ := p.run(ctx); len(c) != 0 {
		t.Fatalf("pass ran twice: %v", c)
	}
}

// Misses (dropped by the earlier passes or never found) go through the
// exact steps too: an id found by barcode or Wikidata becomes the match;
// none leaves the miss, marked as checked.
func TestExactIDsPassMisses(t *testing.T) {
	const (
		found = "f1000000-0000-4000-8000-000000000001"
		none  = "f2000000-0000-4000-8000-000000000002"
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	imgs.KeepID(imgstore.KindArtist, found, srctag.Tag{Source: "deezer"}, "", "")
	imgs.KeepID(imgstore.KindArtist, none, srctag.Tag{Source: "deezer"}, "", "")
	var refetched []string
	p := &exactIDsPass{imgs: imgs, dir: t.TempDir(), now: time.Now, logf: t.Logf,
		brief:  func(context.Context, string) (mbread.Brief, bool) { return mbread.Brief{Name: "Roxana"}, true },
		rivals: func(context.Context, string, string) []string { return []string{"r"} },
		exact: func(_ context.Context, _, mbid, _, _, _ string) (string, string, string, error) {
			if mbid == found {
				return "77", "", imgstore.MatchBarcode, nil
			}
			return "", "", "", nil
		},
		evidence: func(context.Context, string, string, string, string, []string) (imgstore.AppleEvidence, error) {
			t.Fatal("a miss has no name step")
			return imgstore.AppleEvidence{}, nil
		},
		refetch: func(_ context.Context, mbid string) { refetched = append(refetched, mbid) }}
	counts, err := p.run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, found, "deezer"); ext != "77" || imgs.FoundBy(imgstore.KindArtist, found, "deezer") != imgstore.ViaBarcode {
		t.Fatalf("found miss: %q", ext)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, none, "deezer"); ext != "" || imgs.FoundBy(imgstore.KindArtist, none, "deezer") != viaChecked {
		t.Fatalf("none: %q %q", ext, imgs.FoundBy(imgstore.KindArtist, none, "deezer"))
	}
	if counts["deezer barcode found"] != 1 || counts["deezer none missing"] != 1 || len(refetched) != 1 {
		t.Fatalf("counts %v refetched %v", counts, refetched)
	}
}

// The first barcode rule took an album's first artist; its changes are
// undone (the old id back, its pictures refetched) and every barcode row
// is classified again, once.
func TestExactIDsBarcodeRecheck(t *testing.T) {
	const (
		changed = "g1000000-0000-4000-8000-000000000001"
		same    = "g2000000-0000-4000-8000-000000000002"
		apple   = "g3000000-0000-4000-8000-000000000003"
	)
	imgs, err := imgstore.Open(imgstore.Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer imgs.Close()
	imgs.KeepID(imgstore.KindArtist, changed, srctag.Tag{Source: "deezer"}, imgstore.MatchBarcode, "2")
	imgs.SetVia(imgstore.KindArtist, changed, "deezer", imgstore.ViaBarcode)
	imgs.KeepID(imgstore.KindArtist, same, srctag.Tag{Source: "deezer"}, "search", "5")
	imgs.SetVia(imgstore.KindArtist, same, "deezer", imgstore.ViaBarcode)
	dir := t.TempDir()
	lines := `{"mbid":"` + changed + `","source":"deezer","old":"1","new":"2","via":"barcode","outcome":"changed"}
{"mbid":"` + apple + `","source":"applemusic","old":"77","new":"88","via":"barcode","outcome":"changed"}
`
	if err := os.WriteFile(filepath.Join(dir, "exact-ids-20261029-231957.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	var refetched []string
	p := &exactIDsPass{imgs: imgs, dir: dir, now: time.Now, logf: t.Logf,
		refetch: func(_ context.Context, mbid string) { refetched = append(refetched, mbid) }}
	n, err := p.barcodeRecheck(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("restored %d %v", n, err)
	}
	if _, ext, _, _ := imgs.KeptID(imgstore.KindArtist, changed, "deezer"); ext != "1" || imgs.FoundBy(imgstore.KindArtist, changed, "deezer") != "" {
		t.Fatalf("changed: %q", ext)
	}
	if st, ext, _, _ := imgs.KeptID(imgstore.KindArtist, apple, "applemusic"); ext != "77" || st != "us" {
		t.Fatalf("apple: %q %q", st, ext)
	}
	if imgs.FoundBy(imgstore.KindArtist, same, "deezer") != "" {
		t.Fatal("barcode row not opened again")
	}
	if len(refetched) != 2 {
		t.Fatalf("refetched %v", refetched)
	}
	if n, _ := p.barcodeRecheck(context.Background()); n != 0 {
		t.Fatal("ran twice")
	}
}
