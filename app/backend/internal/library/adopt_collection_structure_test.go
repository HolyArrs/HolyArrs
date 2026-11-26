package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestPackStructuralDiscovery(t *testing.T) {
	root := t.TempDir()
	outer := "The Expendables 1, 2, 3 - Sylvester Stallone Action 2010-2014 Eng Sub 720p [H264-mp4]"
	inner := outer + "/The Expendables 1, 2, 3 [H264-mp4]"
	files := []string{
		inner + "/01 The Expendables - Extended Directors Cut Sylvester Stallone 2010 Eng Sub 720p [H264-mp4].mp4",
		inner + "/02 The Expendables 2 - Sylvester Stallone 2012 Eng Sub 720p [H264-mp4].mp4",
		inner + "/03 The Expendables 3 - Extended Unrated Sylvester Stallone 2014 Eng Sub 720p [H264-mp4].mp4",
	}
	for _, f := range files {
		writeFile(t, filepath.Join(root, f), "original video")
	}
	writeFile(t, filepath.Join(root, outer, "Images", "cover.jpg"), "cover")
	writeFile(t, filepath.Join(root, outer, "Subtitles Eng [SubRip - MicroDVD]", "title.srt"), "subtitle")
	es, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := movieCandidates(root, es, nil)
	if len(got) != 3 {
		t.Fatalf("want 3 files, got %q", got)
	}
	for i, f := range files {
		if got[i] != f {
			t.Fatalf("changed candidate: %q", got)
		}
		b, e := os.ReadFile(filepath.Join(root, f))
		if e != nil || string(b) != "original video" {
			t.Fatal("file changed")
		}
	}
}

func TestGenericNestedPackStructure(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"Example 2000-2002/Films/01 Example - Extended Directors Cut Pat Smith 2000 Eng 720p.mkv", "Example 2000-2002/Films/02 Example 2 - Unrated Pat Smith 2002 Eng 720p.mkv"} {
		writeFile(t, filepath.Join(root, f), "video")
	}
	es, _ := os.ReadDir(root)
	got := movieCandidates(root, es, nil)
	if len(got) != 2 {
		t.Fatalf("%q", got)
	}

}

func TestAmbiguousPackCannotAdoptLargestVideo(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	for _, f := range []string{"Unknown Collection/one.mkv", "Unknown Collection/two.mkv"} {
		writeFile(t, filepath.Join(lf.Path, f), "video")
	}
	svc := &Service{Store: s, Metadata: adoptTMDB}
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil || len(p.Folders) != 1 || p.Folders[0].Main != "" || p.Folders[0].Match != nil || p.Folders[0].Why != "Possible collection pack: review its contents and import individual movie files." {
		t.Fatalf("%+v %v", p, err)
	}
	r, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{{Folder: "Unknown Collection", TMDB: "949"}}, domain.System)
	if err != nil || len(r) != 1 || r[0].Error == "" || r[0].TitleID != 0 {
		t.Fatalf("adopted ambiguous pack: %+v %v", r, err)
	}
}

func TestPackGuardPreservesEncodesAndDiscs(t *testing.T) {
	for _, videos := range [][]AdoptVideo{
		{{Path: "Example Collection/Example (2000) 1080p.mkv"}, {Path: "Example Collection/Example (2000) 2160p.mkv"}},
		{{Path: "Example Collection/Disc 1/movie.mkv"}, {Path: "Example Collection/Disc 2/movie.mkv"}},
	} {
		if possibleCollectionPack("Example Collection", videos) {
			t.Fatalf("ordinary movie split: %+v", videos)
		}
	}
}

func TestPackExtrasDoNotDisableGuard(t *testing.T) {
	videos := []AdoptVideo{{Path: "Unknown Collection/one.mkv"}, {Path: "Unknown Collection/two.mkv"}, {Path: "Unknown Collection/Extras/behind scenes.mkv"}}
	if !possibleCollectionPack("Unknown Collection", videos) {
		t.Fatal("extras bypassed collection guard")
	}
}

func TestEmptyTrilogyDoesNotMatchSamples(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Example Trilogy", "sample", "Example (2000)-sample.mp4"), "sample")
	writeFile(t, filepath.Join(lf.Path, "Example Trilogy", "Example (2000).srt"), "subtitle")
	p, err := PlanAdopt(context.Background(), s.Read(), adoptTMDB, lf.ID)
	if err != nil || len(p.Folders) != 1 || p.Folders[0].Status != "no_video" || p.Folders[0].Match != nil || p.Folders[0].Main != "" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestPackLooseMoviesAndDiscsStillGuarded(t *testing.T) {
	vs := []AdoptVideo{{Path: "Example Collection/one.mkv"}, {Path: "Example Collection/two.mkv"}, {Path: "Example Collection/Disc 1/movie.mkv"}}
	if !possibleCollectionPack("Example Collection", vs) {
		t.Fatal("disc bypassed guard for loose movies")
	}
}

func TestYearFirstCollectionDiscovery(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"Example Trilogy/2000.Example.One.1920x1080.BDRip.mkv", "Example Trilogy/2002.Example.Two.1920x1080.BDRip.mkv", "Example Trilogy/2004.Example.Three.1920x1080.BDRip.mkv"} {
		writeFile(t, filepath.Join(root, f), "video")
	}
	es, _ := os.ReadDir(root)
	got := movieCandidates(root, es, nil)
	if len(got) != 3 {
		t.Fatalf("year-first pack: %q", got)
	}
	if _, y := collectionReleaseFacts("2001.A.Space.Odyssey.1968.1920x1080.BDRip"); y != 0 {
		t.Fatal("numeric title mistaken for year")
	}
}
