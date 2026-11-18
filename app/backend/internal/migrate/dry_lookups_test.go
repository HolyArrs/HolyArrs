package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A dry run of 300 shows looks each one up once when its own id finds it,
// and asks for no names; 30 found only by the old app's TMDB id cost a few
// lookups more each.
func TestDryRunLookupsPerShow(t *testing.T) {
	l := newMoveLab(t)
	tv := filepath.Join(t.TempDir(), "tv")
	if err := os.MkdirAll(tv, 0o755); err != nil {
		t.Fatal(err)
	}
	src := &manyShows{}
	sonarr := &Snapshot{Kind: Sonarr, Via: ViaAPI, Folders: []Folder{{Path: "/tv"}}}
	for i := 1; i <= 300; i++ {
		n := strconv.Itoa(i)
		ids := map[string]string{"tvdb": n, "tmdb": "9" + n}
		sh := metadata.Show{Source: "tmdb", Name: "Show " + n, IDs: map[string]string{"tmdb": "9" + n, "tvdb": n}}
		if i%10 == 0 {
			sh.IDs = map[string]string{"tmdb": "9" + n} // TMDB does not know its TVDB id
			sh.Name = "Other " + n
		}
		src.list = append(src.list, sh)
		sonarr.Titles = append(sonarr.Titles, Title{SourceID: int64(i), Kind: "show", Name: "Show " + n, IDs: ids, Path: "/tv/Show " + n, Monitored: true})
		if err := os.MkdirAll(filepath.Join(tv, "Show "+n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": src}
	sonarr.finish()
	saved, err := Save(context.Background(), l.st, 1, sonarr)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	l.rules = append(l.rules, PathRule{From: "/tv", To: filepath.ToSlash(tv)})
	p := l.plan(t)
	start := time.Now()
	rep := l.mv.Run(context.Background(), 0, p, true, domain.Actor{Kind: "user", ID: 1}, nil)
	took := time.Since(start)
	seasons := 0
	for _, ln := range rep.Lines {
		if ln.Attention == AttnSeasons {
			seasons++
		}
	}
	// 270 shows: one lookup each. 30 shows: their TVDB id (1), one name
	// search (1), the TMDB id to read its name (1) and the check the add
	// makes (1).
	t.Logf("300 shows: %d metadata calls in %v; %d found only by the old app's TMDB id", src.calls, took, seasons)
	if seasons != 30 || src.calls > 270+30*4 {
		t.Fatalf("%d calls, %d marked", src.calls, seasons)
	}
}
