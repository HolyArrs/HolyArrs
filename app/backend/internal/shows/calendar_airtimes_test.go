package shows

import (
	"context"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// dayOnly is the lab show the way TMDB lists it: air days only, each
// episode's AirDate the end of its day and untimed.
func dayOnly(sh metadata.Show) metadata.Show {
	sh.Episodes = append([]metadata.Episode(nil), sh.Episodes...)
	for i := range sh.Episodes {
		e := &sh.Episodes[i]
		e.Timed = false
		if e.AirDay != "" {
			d, _ := time.Parse("2006-01-02", e.AirDay)
			end := d.AddDate(0, 0, 1)
			e.AirDate = &end
		}
	}
	return sh
}

// Calendar air times: an episode whose details source knows only the day
// is an all-day entry (timed false, its day), never its day's end shown as
// a time; once TVMaze lists the exact time for the same show, the refresh
// stores that time and the calendar shows it.
func TestCalendarUntimedThenTVMazeTime(t *testing.T) {
	svc, s, tmdb, maze := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	tmdb.set(dayOnly(labShow("tmdb", "100")), "tmdb:100", "tvdb:200", "imdb:tt0100")
	maze.set(dayOnly(labShow("tvmaze", "300")), "tvmaze:300", "tvdb:200", "imdb:tt0100") // TVMaze knows no times either
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	es, err := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 14), library.CalendarShows)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("entries %+v", es)
	}
	for _, e := range es {
		if e.Timed || e.Date.Format("2006-01-02") != e.AirDay || e.Date.Hour() != 0 {
			t.Fatalf("untimed entry %+v", e)
		}
	}
	// Just the day's last range day: a day-only episode on it is listed by
	// its day, though its stored day end falls past the range.
	if es, _ := library.Calendar(ctx, s.Read(), from, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), library.CalendarShows); len(es) != 1 || es[0].AirDay != "2026-10-11" {
		t.Fatalf("last day %+v", es)
	}

	// TVMaze now has the times: the refresh stores them.
	maze.set(labShow("tvmaze", "300"), "tvmaze:300", "tvdb:200", "imdb:tt0100")
	if _, err := svc.RefreshShow(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	es, _ = library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 14), library.CalendarShows)
	if len(es) != 2 || !es[0].Timed || !es[0].Date.Equal(time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)) || es[0].AirDay != "2026-10-11" ||
		!es[1].Timed || !es[1].Date.Equal(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("timed entries %+v", es)
	}
	var tvmazeID string
	if err := s.Read().QueryRowContext(ctx, "SELECT json_extract(source_ids, '$.tvmaze') FROM titles WHERE id = ?", id).Scan(&tvmazeID); err != nil || tvmazeID != "300" {
		t.Fatalf("tvmaze id kept: %q %v", tvmazeID, err)
	}
}
