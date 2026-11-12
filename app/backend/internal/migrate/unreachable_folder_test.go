package migrate

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A title under an old root folder this server cannot see (or one left
// unchecked) still comes over as wanted, into a Library folder here of the
// same kind, and the report says it needs attention. Nothing is skipped.
func TestMoveTitleUnderUnreachableRootComesOverWanted(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Folders = append(radarr.Folders, Folder{Path: "/movies-other"})
		radarr.Titles = append(radarr.Titles, Title{SourceID: 2, Kind: "movie", Name: "The Matrix Reloaded", Year: 2003,
			IDs: map[string]string{"tmdb": "604"}, Path: "/movies-other/The Matrix Reloaded (2003)", Monitored: true, ProfileID: 2})
	})
	l.mv.Library.Metadata = &metadata.Static{Movies: []metadata.Movie{
		{Name: "The Matrix", Year: 1999, IDs: map[string]string{"tmdb": "603"}},
		{Name: "The Matrix Reloaded", Year: 2003, IDs: map[string]string{"tmdb": "604"}},
	}}
	const key = "title:movie:tmdb:604"
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, key); got.Result != ResCreated || !strings.Contains(got.Detail, "needs attention") {
		t.Fatalf("move %+v", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE json_extract(source_ids, '$.tmdb') = '604' AND state = 'active'"); n != 1 {
		t.Fatalf("title not added: %d", n)
	}
}
