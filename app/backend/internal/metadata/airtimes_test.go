package metadata

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Calendar air times: TVMaze's airstamp is the exact air time when TVMaze
// also has the local air time; with no airstamp, or an empty airtime, only
// the day is known and the episode is untimed (its AirDate is the day's end).
func TestTVMazeAirTimesTimedOnlyWhenKnown(t *testing.T) {
	var hits atomic.Int32
	p := &TVMaze{BaseURL: fakeTVMaze(t, &hits).URL, Gap: time.Millisecond}
	s, err := p.Show(context.Background(), "tvmaze:300")
	if err != nil {
		t.Fatal(err)
	}
	if e := ep(s, 1, 1); !e.Timed || !e.AirDate.Equal(at("2025-01-06T02:00:00Z")) {
		t.Fatalf("airstamp present: %+v", e)
	}
	if e := ep(s, 0, 1); e.Timed || !e.AirDate.Equal(at("2025-03-02T00:00:00Z")) || e.AirDay != "2025-03-01" {
		t.Fatalf("no airtime: %+v", e)
	}
	if e := ep(s, 2, 2); e.Timed || e.AirDate != nil {
		t.Fatalf("no day: %+v", e)
	}
	for _, c := range []struct {
		day           string
		airtime, stmp *string
		timed         bool
		want          string
	}{
		{"2026-10-26", str("19:30"), str("2026-10-26T18:30:00+00:00"), true, "2026-10-26T18:30:00Z"},
		{"2026-10-26", nil, str("2026-10-26T18:30:00+00:00"), true, "2026-10-26T18:30:00Z"}, // no airtime field: the stamp is all there is
		{"2026-10-26", str(""), str("2026-10-26T12:00:00+00:00"), false, "2026-10-27T00:00:00Z"},
		{"2026-10-26", str("19:30"), nil, false, "2026-10-27T00:00:00Z"},
		{"2026-10-26", str("19:30"), str("garbage"), false, "2026-10-27T00:00:00Z"},
	} {
		day, got, timed := mazeAir(c.day, c.airtime, c.stmp)
		if day != c.day || timed != c.timed || got == nil || !got.Equal(at(c.want)) {
			t.Fatalf("%+v: %s %v %v", c, day, got, timed)
		}
	}
}

func str(s string) *string { return &s }

// A show whose source lists only days takes TVMaze's exact times: by season
// and number on the same day, or by the one time every episode that day
// shares. Days TVMaze has no time for stay untimed.
func TestAddAirTimes(t *testing.T) {
	day := func(d string) *time.Time { e, _ := dayEnd(d); return e }
	stamp := func(s string) *time.Time { u := at(s); return &u }
	tmdb := Show{Source: "tmdb", IDs: map[string]string{"tmdb": "1", "tvdb": "200"}, Episodes: []Episode{
		{Season: 1, Number: 1, AirDay: "2026-10-26", AirDate: day("2026-10-26")}, // same numbers, same day
		{Season: 1, Number: 2, AirDay: "2026-10-27", AirDate: day("2026-10-27")}, // numbered differently, one time that day
		{Season: 1, Number: 3, AirDay: "2026-10-27", AirDate: day("2026-10-27")},
		{Season: 1, Number: 4, AirDay: "2026-10-28", AirDate: day("2026-10-28")}, // TVMaze has two times that day
		{Season: 1, Number: 5, AirDay: "2026-10-29", AirDate: day("2026-10-29")}, // TVMaze knows no time
		{Season: 1, Number: 6, AirDay: "2026-10-30", AirDate: day("2026-10-30")}, // TVMaze lists it a day later
		{Season: 1, Number: 7},
	}}
	maze := Show{Source: "tvmaze", AirTime: "19:30", TimeZone: "Europe/London", IDs: map[string]string{"tvmaze": "300", "tvdb": "200"}, Episodes: []Episode{
		{Season: 1, Number: 1, AirDay: "2026-10-26", AirDate: stamp("2026-10-26T18:30:00Z"), Timed: true},
		{Season: 2026, Number: 2, AirDay: "2026-10-27", AirDate: stamp("2026-10-27T07:00:00Z"), Timed: true},
		{Season: 2026, Number: 3, AirDay: "2026-10-27", AirDate: stamp("2026-10-27T07:00:00Z"), Timed: true},
		{Season: 2026, Number: 4, AirDay: "2026-10-28", AirDate: stamp("2026-10-28T07:00:00Z"), Timed: true},
		{Season: 2026, Number: 5, AirDay: "2026-10-28", AirDate: stamp("2026-10-28T19:00:00Z"), Timed: true},
		{Season: 1, Number: 5, AirDay: "2026-10-29", AirDate: day("2026-10-29")},
		{Season: 1, Number: 6, AirDay: "2026-10-31", AirDate: stamp("2026-10-31T18:30:00Z"), Timed: true},
	}}
	if !SameShow(tmdb, maze) || SameShow(tmdb, Show{IDs: map[string]string{"tvdb": "201"}}) {
		t.Fatal("SameShow")
	}
	if n := AddAirTimes(&tmdb, maze); n != 3 {
		t.Fatalf("timed %d: %+v", n, tmdb.Episodes)
	}
	want := map[int]string{1: "2026-10-26T18:30:00Z", 2: "2026-10-27T07:00:00Z", 3: "2026-10-27T07:00:00Z"}
	for _, e := range tmdb.Episodes {
		if w, ok := want[e.Number]; ok {
			if !e.Timed || !e.AirDate.Equal(at(w)) {
				t.Fatalf("E%d %+v", e.Number, e)
			}
			continue
		}
		if e.Timed {
			t.Fatalf("E%d must stay untimed: %+v", e.Number, e)
		}
		if e.AirDay != "" && !e.AirDate.Equal(*day(e.AirDay)) {
			t.Fatalf("E%d day end changed: %+v", e.Number, e)
		}
	}
	if tmdb.AirTime != "19:30" || tmdb.TimeZone != "Europe/London" || tmdb.IDs["tvmaze"] != "300" {
		t.Fatalf("show %+v", tmdb)
	}
}
