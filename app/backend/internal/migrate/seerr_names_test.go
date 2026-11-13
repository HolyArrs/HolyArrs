package migrate

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Requests read from Seerr carry only ids; the import looks their names up
// as a request made in HolyArrs does: a movie by its TMDB id, a show by its
// TVDB id.
func TestMoveGivesSeerrRequestsTheirNames(t *testing.T) {
	l := newMoveLabWith(t, func(_, _, seerr *Snapshot) {
		seerr.Requests = append(seerr.Requests,
			Request{SourceID: 8, Kind: "show", IDs: map[string]string{"tvdb": "200"}, State: "pending", By: 2, ByName: "Ana", Created: "2026-09-01T10:00:00Z"})
	})
	md := l.mv.Library.Metadata.(*metadata.Static)
	md.Movies = append(md.Movies, metadata.Movie{Name: "The Matrix Reloaded", Year: 2003, Poster: "/p/reloaded.jpg", IDs: map[string]string{"tmdb": "604"}})
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": oneShow{metadata.Show{Source: "tmdb", Name: "Lab Show", Year: 2025, Poster: "/p/lab.jpg", IDs: map[string]string{"tmdb": "100", "tvdb": "200"}}}}
	// The preview still shows the id: Seerr's own data has no name for it.
	if got := lineOf(t, l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil), "ask:seerr:7"); got.Name != "TMDB 604" {
		t.Fatalf("preview name %q", got.Name)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	for ref, want := range map[string]string{"tmdb:604": "The Matrix Reloaded|2003|/p/reloaded.jpg", "tvdb:200": "Lab Show|2025|/p/lab.jpg"} {
		var got string
		if err := l.st.Read().QueryRow("SELECT name || '|' || year || '|' || poster FROM asks WHERE ref = ?", ref).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: %q, %v; want %q", ref, got, err, want)
		}
	}
	if got := lineOf(t, mv.Report, "ask:seerr:8"); got.Name != "Lab Show" {
		t.Fatalf("report line %+v", got)
	}
}

// When the details source cannot answer, the request keeps its stand-in
// name; the Requests fill-in finds the real one later.
func TestMoveKeepsStandInNameWhenLookupFails(t *testing.T) {
	l := newMoveLab(t) // TMDB 604 is unknown to the details source
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move %+v", mv)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE ref = 'tmdb:604' AND name = 'TMDB 604'"); n != 1 {
		t.Fatal("stand-in name not kept")
	}
}
