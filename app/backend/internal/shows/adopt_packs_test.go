package shows

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

func TestTVPackNames(t *testing.T) {
	for _, tc := range []struct {
		raw, name string
		year      int
	}{
		{"Parks and Recreation Complete (1080p WEB-DL HEVC x254 10bit AAC 5.1 ImE) [UTR]", "Parks and Recreation", 0},
		{"Peaky Blinders (2013) Season 1-6 S01-S06 + Extras (1080p BluRay x265 HEVC 10bit EAC3 Mixed Ghost)", "Peaky Blinders", 2013},
		{"Example.Comedy.Complete (1080p WEB-DL)", "Example Comedy", 0},
		{"Example Drama (1996-99)", "Example Drama", 1996},
		{"The Office (US)", "The Office (US)", 0},
		{"24", "24", 0},
		{"Complete Savages", "Complete Savages", 0},
		{"The Complete Guide", "The Complete Guide", 0},
	} {
		n, y := showFolderName(tc.raw)
		if n != tc.name || y != tc.year {
			t.Errorf("%q: %q %d, want %q %d", tc.raw, n, y, tc.name, tc.year)
		}
	}
}

func TestTVCollectionDiscoveryAndAdoption(t *testing.T) {
	svc, store, _, _ := addShowService(t)
	lf := addShowFolder(t, store)
	ctx := context.Background()
	for _, rel := range []string{
		"Series (Pack 3)/1a. Lab Show (2025)/Season 1/Lab.Show.S01E01.mkv",
		"Series (Pack 3)/2b. Another Show (2024)/Season 1/Another.S01E01.mkv",
		"Series (Pack 3)/Extras/Bonus.S01E01.mkv",
		"Lab Show Complete/Season 01/Lab.Show.S01E01.mkv",
		"Lab Show Complete/Season 02/Lab.Show.S02E01.mkv",
		"Lab Show Collection/Season 1/Lab.Show.S01E01.mkv",
		"Lab Show Collection/Season 2/Lab.Show.S02E01.mkv",
	} {
		writeFile(t, filepath.Join(lf.Path, filepath.FromSlash(rel)), "video")
	}
	before := snapshot(t, lf.Path)
	p, err := PlanShowAdoptFrom(ctx, store.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Folders) != 4 {
		t.Fatalf("candidates: %+v", p.Folders)
	}
	name := "Series (Pack 3)/1a. Lab Show (2025)"
	c := byFolder(p)[name]
	if c.Match == nil || c.Match.ID != "tmdb:100" || c.Episodes != 1 {
		t.Fatalf("nested show: %+v", c)
	}
	next, err := PlanShowAdoptFrom(ctx, store.Read(), svc.Shows, lf.ID, name)
	if err != nil || next.Offset != 3 || len(next.Folders) != 1 {
		t.Fatalf("resume: %+v %v", next, err)
	}
	r, err := svc.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: name, ID: "tmdb:100"}}, domain.System)
	if err != nil || r[0].Error != "" || r[0].Episodes != 1 {
		t.Fatalf("adopt: %+v %v", r, err)
	}
	for _, overlap := range []string{"Series (Pack 3)", name + "/Season 1"} {
		err := store.Write(ctx, func(tx *sql.Tx) error { return tvFolderAvailable(ctx, tx, lf.ID, overlap, false) })
		if err == nil {
			t.Fatalf("accepted overlapping folder %q", overlap)
		}
	}
	if !reflect.DeepEqual(before, snapshot(t, lf.Path)) {
		t.Fatal("media changed")
	}
	p, err = PlanShowAdoptFrom(ctx, store.Read(), svc.Shows, lf.ID, "")
	if err != nil || byFolder(p)[name].Status != "in_library" {
		t.Fatalf("rescan: %+v %v", p, err)
	}
}

func TestTVFolderSafety(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pack", "show"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../outside", "pack/../pack/show", "pack/.hidden", "/tmp", `pack\show`} {
		if validTVFolder(root, rel) {
			t.Errorf("accepted %q", rel)
		}
	}
	if !validTVFolder(root, "pack/show") {
		t.Fatal("nested folder rejected")
	}
	if err := os.Symlink(filepath.Join(root, "pack"), filepath.Join(root, "link")); err == nil && validTVFolder(root, "link/show") {
		t.Fatal("accepted symlink ancestor")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ents, _ := os.ReadDir(root)
	if _, err := tvFolders(ctx, root, ents, nil, false); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestTVPackBonusCannotClaimEpisodes(t *testing.T) {
	_, _, provider, _ := addShowService(t)
	sh, err := provider.Show(context.Background(), "tmdb:100")
	if err != nil {
		t.Fatal(err)
	}
	files := mapEpisodeFiles([]library.AdoptVideo{{Path: "Collection/Show/Extras/Bonus.S01E01.mkv", Size: 1000}, {Path: "Collection/Show/Season 1/Show.S01E01.mkv", Size: 100}}, sh, "aired")
	for _, f := range files {
		if tvExtraPath(f.Path) && len(f.Keys) > 0 {
			t.Fatal("bonus claimed an episode")
		}
		if !tvExtraPath(f.Path) && len(f.Keys) != 1 {
			t.Fatalf("main episode not assigned: %+v", f)
		}
	}
}

func TestTVCollectionKeepsKnownOwnerAndSeasonFolders(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"Collection Pack/Show A/a.mkv", "Collection Pack/Show B/b.mkv", "Show Collection/Season 1/a.mkv", "Show Collection/Specials/S00E01.mkv"} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "v")
	}
	ents, _ := os.ReadDir(root)
	names, err := tvFolders(context.Background(), root, ents, map[string]int64{"Collection Pack": 1}, false)
	if err != nil || !reflect.DeepEqual(names, []string{"Collection Pack", "Show Collection"}) {
		t.Fatalf("%v %v", names, err)
	}
	if tvHasMainVideo([]library.AdoptVideo{{Path: "Pack/Show/Extras/Intro.mp4"}}) {
		t.Fatal("extras-only folder presented as full episodes")
	}
	if !tvHasMainVideo([]library.AdoptVideo{{Path: "Show/Specials/S00E01.mkv"}}) {
		t.Fatal("numbered specials excluded")
	}
}
