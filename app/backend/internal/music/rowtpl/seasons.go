package rowtpl

import (
	"sort"
	"time"
)

// south lists the regions where the seasons are flipped.
var south = map[string]bool{"au": true, "nz": true, "za": true, "ar": true, "br": true, "cl": true, "uy": true, "py": true, "bo": true, "pe": true}

// lunarDates are the Lunar New Year days. They cannot be computed here, so the table must always cover the next two years.
var lunarDates = []time.Time{
	date(2026, 2, 17), date(2027, 2, 6), date(2028, 1, 26), date(2029, 2, 13), date(2030, 2, 3),
	date(2031, 1, 23), date(2032, 2, 11), date(2033, 1, 31), date(2034, 2, 19), date(2035, 2, 8),
}

// SeasonKeys are the seasons in Browse order, before date ordering.
var SeasonKeys = []string{"halloween", "christmas", "new-year", "valentines", "summer", "dia-de-muertos", "lunar-new-year", "pride", "thanksgiving", "back-to-school", "fall", "spring"}

// Season is one celebration or season with its window in a year.
type Season struct {
	Key   string
	Start time.Time
	End   time.Time
}

// SeasonActive reports whether the season is on at t in the region.
func SeasonActive(key string, t time.Time, region string) bool {
	_, on := window(key, t, region)
	return on
}

// SeasonOrder answers the seasons for Browse: active ones, ending soonest first, then those starting within 45 days, then the rest by next start.
func SeasonOrder(t time.Time, region string) []Season {
	type x struct {
		s     Season
		group int
	}
	d := dayOf(t)
	var l []x
	for _, k := range SeasonKeys {
		w, on := window(k, t, region)
		g := 2
		if on {
			g = 0
		} else if w.Start.Sub(d) <= 45*24*time.Hour {
			g = 1
		}
		l = append(l, x{w, g})
	}
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].group != l[j].group {
			return l[i].group < l[j].group
		}
		if l[i].group == 0 {
			return l[i].s.End.Before(l[j].s.End)
		}
		return l[i].s.Start.Before(l[j].s.Start)
	})
	out := make([]Season, len(l))
	for i, v := range l {
		out[i] = v.s
	}
	return out
}

// date makes a midnight UTC day.
func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// nthWeekday is the n-th given weekday of a month.
func nthWeekday(y int, m time.Month, wd time.Weekday, n int) time.Time {
	t := date(y, m, 1)
	for t.Weekday() != wd {
		t = t.AddDate(0, 0, 1)
	}
	return t.AddDate(0, 0, 7*(n-1))
}

// windowsIn answers a season's windows that touch year y. A window can start the year before, like New Year.
func windowsIn(key string, y int, region string) []Season {
	s := south[region]
	w := func(sm time.Month, sd int, em time.Month, ed int, yy int) Season {
		st := date(yy, sm, sd)
		en := date(yy, em, ed)
		if en.Before(st) {
			en = en.AddDate(1, 0, 0)
		}
		return Season{key, st, en}
	}
	var out []Season
	for _, yy := range []int{y - 1, y} {
		switch key {
		case "halloween":
			out = append(out, w(10, 1, 10, 31, yy))
		case "dia-de-muertos":
			out = append(out, w(10, 20, 11, 2, yy))
		case "thanksgiving":
			if region == "ca" {
				out = append(out, Season{key, date(yy, 9, 25), nthWeekday(yy, 10, time.Monday, 2)})
			} else {
				out = append(out, Season{key, date(yy, 11, 1), nthWeekday(yy, 11, time.Thursday, 4)})
			}
		case "christmas":
			out = append(out, w(11, 15, 12, 26, yy))
		case "new-year":
			out = append(out, w(12, 26, 1, 7, yy))
		case "lunar-new-year":
			for _, d := range lunarDates {
				if d.Year() == yy+1 || d.Year() == yy {
					out = append(out, Season{key, d.AddDate(0, 0, -21), d.AddDate(0, 0, 7)})
				}
			}
		case "valentines":
			out = append(out, w(2, 1, 2, 14, yy))
		case "pride":
			out = append(out, w(6, 1, 6, 30, yy))
		case "spring":
			if s {
				out = append(out, w(9, 22, 12, 20, yy))
			} else {
				out = append(out, w(3, 20, 6, 20, yy))
			}
		case "summer":
			if s {
				out = append(out, w(12, 21, 3, 19, yy))
			} else {
				out = append(out, w(6, 21, 9, 22, yy))
			}
		case "back-to-school":
			if s {
				out = append(out, w(1, 20, 2, 28, yy))
			} else {
				out = append(out, w(8, 10, 9, 15, yy))
			}
		case "fall":
			if s {
				out = append(out, w(3, 20, 6, 20, yy))
			} else {
				out = append(out, w(9, 23, 12, 20, yy))
			}
		}
	}
	return out
}

// dayOf cuts a time down to its UTC day.
func dayOf(t time.Time) time.Time { return date(t.Year(), t.Month(), t.Day()) }

// window answers the season's window active on day t, or else the next one to start.
func window(key string, t time.Time, region string) (Season, bool) {
	d := dayOf(t)
	var next Season
	found := false
	for _, y := range []int{d.Year(), d.Year() + 1, d.Year() + 2} {
		for _, w := range windowsIn(key, y, region) {
			if !d.Before(w.Start) && !d.After(w.End) {
				return w, true
			}
			if w.Start.After(d) && (!found || w.Start.Before(next.Start)) {
				next, found = w, true
			}
		}
	}
	return next, false
}
