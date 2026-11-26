package shows

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// #682: a show with hundreds of episode files adopts in one go under a
// short write cap. Every header is read before the write, so the write
// only records rows and stays well inside its cap.
func TestAdoptLongShowStaysInsideWriteCap(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{WriteCap: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	const n = 240
	sh := labShow("tmdb", "100")
	sh.Seasons = []metadata.Season{{Number: 1, Name: "Season 1"}}
	sh.Episodes = nil
	for i := 1; i <= n; i++ {
		sh.Episodes = append(sh.Episodes, metadata.Episode{Season: 1, Number: i, Name: fmt.Sprint("Ep ", i), AirDate: tm("2025-01-06T02:00:00Z"), AirDay: "2025-01-05", ID: fmt.Sprint("tmdb-1", i)})
	}
	tmdb := &fakeShows{}
	tmdb.set(sh, "tmdb:100", "tvdb:200", "imdb:tt0100")
	svc := New(&library.Service{Store: s, Queue: jobs.New(s, jobs.Options{}), Shows: metadata.ShowSources{"tmdb": tmdb, "tvmaze": &fakeShows{}, "tvdb": &fakeShows{err: metadata.ErrNoTVDBKey}},
		Now: func() time.Time { return now }})
	lf := addShowFolder(t, s)
	for i := 1; i <= n; i++ {
		writeInspectionMP4(t, filepath.Join(lf.Path, "Lab Show (2025)", "Season 1", fmt.Sprintf("Lab.Show.S01E%03d.1080p.WEB-DL.H264-GRP.mp4", i)))
	}
	res, err := svc.Adopt(context.Background(), lf.ID, []library.AdoptChoice{{Folder: "Lab Show (2025)", ID: "tmdb:100"}}, domain.System)
	if err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if res[0].Episodes != n {
		t.Fatalf("adopted %d episodes, want %d (unknown %v)", res[0].Episodes, n, res[0].Unknown)
	}
}
