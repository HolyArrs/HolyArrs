package shows

import (
	"context"
	"database/sql"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// 5.2.7: the calendar lists episodes by their exact air time (the browser
// shows the local day), narrows to movies or shows, and leaves out the
// episodes of a deleted show.
func TestCalendarEpisodes(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	es, err := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 14))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("entries %+v", es)
	}
	e := es[0]
	if e.Kind != "episode" || e.TitleID != id || *e.Season != 2 || *e.Episode != 1 || e.EpisodeName != "Return" || !e.Timed ||
		!e.Date.Equal(time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)) || e.AirDay != "2026-10-11" || e.Network != "Lab TV" || e.State == "" {
		t.Fatalf("episode %+v", e)
	}
	if es[1].EpisodeName != "Next Week" {
		t.Fatalf("second %+v", es[1])
	}
	if ms, _ := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 14), library.CalendarMovies); len(ms) != 0 {
		t.Fatalf("movies only %+v", ms)
	}
	if ss, _ := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 14), library.CalendarShows); len(ss) != 2 {
		t.Fatalf("shows only %+v", ss)
	}
	if _, err := svc.Remove(ctx, id, library.RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if es, _ := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 14)); len(es) != 0 {
		t.Fatalf("a deleted show's episodes %+v", es)
	}
}

// 5.2 review: an air day is a local date; a range that does not start at
// midnight UTC lists the episodes of the days it covers, no more.
func TestCalendarAirDayInLocalRange(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET air_date = NULL, air_day = '2026-10-13' WHERE title_id = ? AND name = 'Next Week'", id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	plus2 := time.FixedZone("plus2", 2*3600)
	for _, c := range []struct {
		day  int
		want bool
	}{{12, false}, {13, true}, {14, false}} {
		from := time.Date(2026, 10, c.day, 0, 0, 0, 0, plus2)
		es, err := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 1), library.CalendarShows)
		if err != nil {
			t.Fatal(err)
		}
		got := false
		for _, e := range es {
			got = got || e.EpisodeName == "Next Week"
		}
		if got != c.want {
			t.Errorf("local day %d: listed %v, want %v (%+v)", c.day, got, c.want, es)
		}
	}
}

type countingReader struct {
	db   *sql.DB
	rows int
	one  int
}

func (c *countingReader) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	c.rows++
	return c.db.QueryContext(ctx, q, args...)
}

func (c *countingReader) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	c.one++
	return c.db.QueryRowContext(ctx, q, args...)
}

// 4.8.11 review: the calendar's episodes are read with a fixed number of
// queries (items, shows, Slots), however many episodes the range holds.
func TestCalendarEpisodesQueriesDoNotGrow(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	if _, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{3, 3650} {
		from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		if days == 3 {
			from = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
		}
		c := &countingReader{db: s.Read()}
		es, err := library.CalendarEpisodes(ctx, c, from, from.AddDate(0, 0, days))
		if err != nil {
			t.Fatal(err)
		}
		if len(es) == 0 || c.rows != 3 || c.one != 0 {
			t.Fatalf("%d days: %d episodes, %d queries, %d single-row queries", days, len(es), c.rows, c.one)
		}
		for _, e := range es {
			if e.State == "" {
				t.Fatalf("no state %+v", e)
			}
		}
	}
}
