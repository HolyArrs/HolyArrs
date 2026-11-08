package api

import "github.com/HolyArrs/holyarrs/app/backend/internal/library"

// calendarTitles reads the titles filter of the calendar: all (the
// default), movies, shows or music.
func calendarTitles(v string) (string, bool) {
	switch v {
	case "", library.CalendarAll:
		return library.CalendarAll, true
	case library.CalendarMovies, library.CalendarShows, library.CalendarMusic:
		return v, true
	}
	return "", false
}
