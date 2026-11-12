package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func init() {
	UseLanguages(func(s string) string {
		return map[string]string{"en": "en", "eng": "en", "english": "en", "fr": "fr", "french": "fr", "de": "de"}[s]
	})
}

func targetsOf(sf []sideFile) string {
	var out []string
	for _, f := range sf {
		out = append(out, filepath.Base(f.target))
	}
	return strings.Join(out, ",")
}

func TestSideFilesNameRule(t *testing.T) {
	dir := "/dl/Movie.2020.1080p"
	video := dir + "/Movie.2020.1080p.mkv"
	got := sideFilesFor(video, "/lib/Movie (2020)/Movie (2020).mkv", []string{
		dir + "/Movie.2020.1080p.eng.srt", dir + "/Movie.2020.1080p.fr.forced.srt", dir + "/Other.srt", dir + "/Movie.2020.1080p.nfo",
	}, false, false, []string{"srt"})
	if targetsOf(got) != "Movie (2020).en.srt,Movie (2020).fr.forced.srt" {
		t.Fatalf("got %s", targetsOf(got))
	}
}

func TestSideFilesEpisodeRuleAndLanguageOnlyPack(t *testing.T) {
	dir := "/dl/Show.S01.1080p"
	video := dir + "/Show.S01E02.1080p.mkv"
	cands := []string{dir + "/Show.S01E02.English.srt", dir + "/Show.S01E03.English.srt", dir + "/English.srt"}
	got := sideFilesFor(video, "/lib/Show/Season 01/Show - S01E02.mkv", cands, false, true, []string{"srt"})
	if targetsOf(got) != "Show - S01E02.en.srt" {
		t.Fatalf("got %s", targetsOf(got))
	}
}

func TestSideFilesSingleVideoAndSubsFolder(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"Movie.mkv", "Sample/movie-sample.mkv", "Subs/2_English.srt", "Subs/3_English.srt", "Subs/4_French.srt", "readme.txt"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cands, real, all := sideCandidates(dir)
	if real != 1 || all != 1 {
		t.Fatalf("videos %d of %d", real, all) // the Sample folder is not walked
	}
	got := sideFilesFor(filepath.Join(dir, "Movie.mkv"), "/lib/M/M.mkv", cands, true, false, []string{"srt"})
	if targetsOf(got) != "M.en.srt,M.en.1.srt,M.fr.srt" {
		t.Fatalf("got %s", targetsOf(got))
	}
	// Without the single-video rule, names that do not match are not taken.
	if got := sideFilesFor(filepath.Join(dir, "Movie.mkv"), "/lib/M/M.mkv", cands, false, false, []string{"srt"}); len(got) != 0 {
		t.Fatalf("got %s", targetsOf(got))
	}
}

func sideStore(t *testing.T) *database.Store {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSideFilesSettings(t *testing.T) {
	ctx := context.Background()
	s := sideStore(t)
	if got := ReadSideFiles(ctx, s.Read()); got.On || strings.Join(got.Types, ",") != "srt" {
		t.Fatalf("default %+v", got)
	}
	if _, err := SaveSideFiles(ctx, s, SideFiles{On: true, Types: []string{"SRT", ".ass", "srt"}}, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := ReadSideFiles(ctx, s.Read()); !got.On || strings.Join(got.Types, ",") != "srt,ass" {
		t.Fatalf("saved %+v", got)
	}
	for _, bad := range [][]string{{}, {"mkv"}, {"s/rt"}, {"toolongtype"}} {
		if _, err := SaveSideFiles(ctx, s, SideFiles{Types: bad}, domain.System); !errors.Is(err, ErrSideFileTypes) {
			t.Fatalf("%v: %v", bad, err)
		}
	}
}

// Placing: off does nothing; on links or copies each side file beside its
// video and never replaces a file already in the Library.
func TestPlaceSideFiles(t *testing.T) {
	ctx := context.Background()
	s := sideStore(t)
	dl, lib := t.TempDir(), t.TempDir()
	video := filepath.Join(dl, "Movie.2020.mkv")
	for _, f := range []string{video, filepath.Join(dl, "Movie.2020.en.srt"), filepath.Join(dl, "Movie.2020.fr.srt")} {
		if err := os.WriteFile(f, []byte(filepath.Base(f)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(lib, "Movie (2020).mkv")
	if err := os.WriteFile(target, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(lib, "Movie (2020).fr.srt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	copied := 0
	im := &Importer{Store: s, Ops: Ops{Link: func(string, string) error { return errors.New("another disk") }, Copy: func(src, dst string) error {
		copied++
		return copyFile(src, dst)
	}}}
	plan := []mediafiles.PlanStep{{SourceFile: video, TargetPath: target, Method: "hardlink"}}
	res := []mediafiles.PlaceResult{{SourceFile: video, Outcome: "placed", PlacedBy: "copy"}}
	im.placeSideFiles(ctx, dl, plan, res, false)
	if _, err := os.Stat(filepath.Join(lib, "Movie (2020).en.srt")); err == nil {
		t.Fatal("placed while off")
	}
	if _, err := SaveSideFiles(ctx, s, SideFiles{On: true, Types: []string{"srt"}}, domain.System); err != nil {
		t.Fatal(err)
	}
	im.placeSideFiles(ctx, dl, plan, res, false)
	if b, err := os.ReadFile(filepath.Join(lib, "Movie (2020).en.srt")); err != nil || string(b) != "Movie.2020.en.srt" {
		t.Fatalf("en %q %v", b, err)
	}
	if b, _ := os.ReadFile(keep); string(b) != "mine" {
		t.Fatalf("the Library file was replaced: %q", b)
	}
	if copied != 1 {
		t.Fatalf("copied %d", copied)
	}
	// On the same disk it is linked.
	linked := 0
	im.Ops = Ops{Link: func(a, b string) error { linked++; return os.Link(a, b) }}
	_ = os.Remove(filepath.Join(lib, "Movie (2020).en.srt"))
	im.placeSideFiles(ctx, dl, plan, res, false)
	if linked != 1 {
		t.Fatalf("linked %d", linked)
	}
}
