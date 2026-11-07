package metadata

import (
	"strconv"
	"time"
)

// SameShow reports whether two looked-up shows are the same show: they
// share a TVDB or IMDb id (TVMaze lists both).
func SameShow(a, b Show) bool {
	for _, k := range []string{"tvdb", "imdb"} {
		if a.IDs[k] != "" && a.IDs[k] == b.IDs[k] {
			return true
		}
	}
	return false
}

// AddAirTimes gives the episodes of sh that only have an air day (TMDB and
// TVDB list days, not times) the exact air time TVMaze lists for the same
// episode, and returns how many it timed. An episode takes a time when
// TVMaze has the same season and number airing on the same day, or when
// every episode TVMaze lists that day airs at one and the same time (daily
// shows and whole-season drops number their episodes differently). An
// episode TVMaze knows no time for stays untimed. maze must be the same
// show (see SameShow). The show's usual air time and zone, and its TVMaze
// id, are filled in when sh has none.
func AddAirTimes(sh *Show, maze Show) int {
	type key struct{ season, number int }
	byNumber := map[key]Episode{}
	byDay := map[string][]Episode{}
	for _, e := range maze.Episodes {
		if e.AirDay == "" {
			continue
		}
		if e.Season > 0 {
			byNumber[key{e.Season, e.Number}] = e
		}
		byDay[e.AirDay] = append(byDay[e.AirDay], e)
	}
	oneTime := func(day string) *time.Time {
		eps := byDay[day]
		if len(eps) == 0 {
			return nil
		}
		for _, e := range eps {
			if !e.Timed || e.AirDate == nil || !e.AirDate.Equal(*eps[0].AirDate) {
				return nil
			}
		}
		return eps[0].AirDate
	}
	n := 0
	for i := range sh.Episodes {
		e := &sh.Episodes[i]
		if e.Timed || e.AirDay == "" {
			continue
		}
		var at *time.Time
		if m, ok := byNumber[key{e.Season, e.Number}]; ok && e.Season > 0 && m.Timed && m.AirDate != nil && m.AirDay == e.AirDay {
			at = m.AirDate
		} else {
			at = oneTime(e.AirDay)
		}
		if at == nil {
			continue
		}
		t := at.UTC()
		e.AirDate, e.Timed = &t, true
		n++
	}
	if sh.AirTime == "" && maze.AirTime != "" {
		sh.AirTime, sh.TimeZone = maze.AirTime, maze.TimeZone
	}
	if id := maze.IDs["tvmaze"]; id != "" {
		if sh.IDs == nil {
			sh.IDs = map[string]string{}
		}
		if _, err := strconv.ParseInt(id, 10, 64); err == nil && sh.IDs["tvmaze"] == "" {
			sh.IDs["tvmaze"] = id
		}
	}
	return n
}
