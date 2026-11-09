package library

import (
	"context"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// 4.15.5: the Calendar lists the release days of an artist's included
// albums with how many tracks are in the Library; music narrows it to
// albums, movies and shows leave them out.
func TestCalendarAlbums(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	id, _ := refreshedLabArtist(t, svc)
	from := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(1, 0, 0)
	es, err := Calendar(ctx, s.Read(), from, to, CalendarMusic)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 || es[0].Kind != "album" || es[0].Album != "Old Days" || es[0].Name != "Lab Band" || es[0].TitleID != id ||
		!es[0].Date.Equal(time.Date(2010, 5, 1, 0, 0, 0, 0, time.UTC)) || es[0].Tracks != 3 || es[0].Have != 0 || es[0].PartID == 0 {
		t.Fatalf("%+v", es)
	}
	all, _ := Calendar(ctx, s.Read(), from, to)
	if len(all) != 1 {
		t.Fatalf("all %+v", all)
	}
	for _, w := range []string{CalendarMovies, CalendarShows} {
		if es, _ := Calendar(ctx, s.Read(), from, to, w); len(es) != 0 {
			t.Fatalf("%s: %+v", w, es)
		}
	}
	// A deleted artist leaves the Calendar.
	if _, err := svc.Remove(ctx, id, RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if es, _ := Calendar(ctx, s.Read(), from, to, CalendarMusic); len(es) != 0 {
		t.Fatalf("removed %+v", es)
	}
}

// 4.15 review: an album dated only by year (Middle, 2018) or month (Short,
// February 2019) has no release day, so the Calendar leaves it out; its stored
// date keeps the release-gate rule (end of that year or month).
func TestCalendarLeavesOutAlbumsWithNoDay(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	refreshedLabArtist(t, svc)
	from := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	es, err := Calendar(ctx, s.Read(), from, from.AddDate(2, 0, 0), CalendarMusic)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.Album == "Middle" || e.Album == "Short" {
			t.Fatalf("an album with no release day is listed: %+v", e)
		}
	}
	var prec map[string]string = map[string]string{}
	rows, _ := s.Read().Query("SELECT name, coalesce(release_date_precision, ''), substr(release_date, 1, 10) FROM parts WHERE kind = 'album'")
	defer rows.Close()
	for rows.Next() {
		var n, p, d string
		_ = rows.Scan(&n, &p, &d)
		prec[n] = p + " " + d
	}
	if prec["Middle"] != "year 2018-12-31" || prec["Short"] != "month 2019-02-28" || prec["Old Days"] != "day 2010-05-01" {
		t.Fatalf("precision %v", prec)
	}
}
