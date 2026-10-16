package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// The calendar feed has its own key, not a session or an API key: it can
// only read the calendar. Only its SHA-256 is stored; the key is shown once.
const (
	feedKeyHash = "calendar_feed_key_hash"
	feedKeyAt   = "calendar_feed_key_at"
	// FeedPath is the iCal feed; it is open without sign-in and checks the key.
	FeedPath = "/api/v1/calendar/feed.ics"
)

func (a *API) feedRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/calendar/feed.ics", a.calendarFeed)
	m.HandleFunc("GET /api/v1/calendar/feed-key", a.getFeedKey)
	m.HandleFunc("POST /api/v1/calendar/feed-key", a.newFeedKey)
	m.HandleFunc("DELETE /api/v1/calendar/feed-key", a.deleteFeedKey)
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

func (a *API) feedSetting(r *http.Request, key string) string {
	var v string
	_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	return v
}

func (a *API) getFeedKey(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"set": a.feedSetting(r, feedKeyHash) != "", "path": FeedPath}
	if at := a.feedSetting(r, feedKeyAt); at != "" {
		out["created_at"] = at
	}
	writeJSON(w, http.StatusOK, out)
}

// newFeedKey makes a new feed key (the old one stops working) and shows it
// this once.
func (a *API) newFeedKey(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		internal(w, err)
		return
	}
	key := base64.RawURLEncoding.EncodeToString(b)
	now := store.FormatTime(time.Now())
	by := actor(r)
	byText, _ := json.Marshal(by)
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		for k, v := range map[string]string{feedKeyHash: hashKey(key), feedKeyAt: now} {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, string(byText), now, string(byText)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		internal(w, err)
		return
	}
	_ = a.s.Store.AddEvent(r.Context(), &domain.Event{Actor: by, Kind: domain.EventFeedKeyChanged, Summary: "Made a new calendar feed key; the old link stops working"})
	writeJSON(w, http.StatusCreated, map[string]any{"key": key, "path": FeedPath + "?key=" + key, "created_at": now})
}

func (a *API) deleteFeedKey(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireStrong(w, r) {
		return
	}
	err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM settings WHERE grp = 'this_app' AND key IN (?, ?)", feedKeyHash, feedKeyAt)
		return err
	})
	if err != nil {
		internal(w, err)
		return
	}
	_ = a.s.Store.AddEvent(r.Context(), &domain.Event{Actor: actor(r), Kind: domain.EventFeedKeyChanged, Summary: "Turned the calendar feed off"})
	w.WriteHeader(http.StatusNoContent)
}

// calendarFeed answers the iCal feed: every cinema, digital and physical
// date from past_days ago (default 7) to future_days ahead (default 28) as
// all-day events. kinds narrows it (cinema,digital,physical).
func (a *API) calendarFeed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	want := a.feedSetting(r, feedKeyHash)
	got := q.Get("key")
	w.Header().Set("Cache-Control", "no-store")
	if want == "" || got == "" || subtle.ConstantTimeCompare([]byte(hashKey(got)), []byte(want)) != 1 {
		http.Error(w, "the calendar feed key is wrong or the feed is off", http.StatusUnauthorized)
		return
	}
	days := func(name string, def int) (int, bool) {
		v := q.Get(name)
		if v == "" {
			return def, true
		}
		n, err := strconv.Atoi(v)
		return n, err == nil && n >= 0 && n <= 366
	}
	past, ok1 := days("past_days", 7)
	future, ok2 := days("future_days", 28)
	if !ok1 || !ok2 {
		http.Error(w, "past_days and future_days are 0 to 366", http.StatusBadRequest)
		return
	}
	titles, ok := calendarTitles(q.Get("titles"))
	if !ok {
		http.Error(w, "titles must be all, movies, shows or music", http.StatusBadRequest)
		return
	}
	kinds := map[string]bool{"cinema": true, "digital": true, "physical": true, "episode": true, "album": true}
	if v := q.Get("kinds"); v != "" {
		kinds = map[string]bool{}
		for _, k := range strings.Split(v, ",") {
			kinds[strings.TrimSpace(k)] = true
		}
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	es, err := library.Calendar(r.Context(), a.s.Store.Read(), today.AddDate(0, 0, -past), today.AddDate(0, 0, future+1), titles)
	if err != nil {
		internal(w, err)
		return
	}
	var keep []library.CalendarEntry
	for _, e := range es {
		if kinds[e.Kind] {
			keep = append(keep, e)
		}
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	_, _ = w.Write([]byte(ICal(keep, time.Now().UTC())))
}

var dateWords = map[string]string{"cinema": "In cinemas", "digital": "Digital release", "physical": "Disc release", "episode": "Episode", "album": "Album release"}

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

// ICal writes calendar entries as an iCalendar file of all-day events.
func ICal(es []library.CalendarEntry, now time.Time) string {
	var b strings.Builder
	line := func(s string) {
		// Fold at 75 octets, never inside a UTF-8 character.
		for len(s) > 75 {
			cut := 75
			for cut > 0 && s[cut]&0xC0 == 0x80 {
				cut--
			}
			b.WriteString(s[:cut] + "\r\n")
			s = " " + s[cut:]
		}
		b.WriteString(s + "\r\n")
	}
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//HolyArrs//Calendar//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:PUBLISH")
	line("X-WR-CALNAME:HolyArrs")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, e := range es {
		if e.Kind == "episode" {
			// An episode is a timed event at its air time (UTC) for its
			// running time, or an all-day event on its air day.
			what := e.Name
			if e.Season != nil && e.Episode != nil {
				what += fmt.Sprintf(" - S%02dE%02d", *e.Season, *e.Episode)
			}
			if e.EpisodeName != "" {
				what += " - " + e.EpisodeName
			}
			line("BEGIN:VEVENT")
			line(fmt.Sprintf("UID:holyarrs-episode-%d@holyarrs", e.ItemID))
			line("DTSTAMP:" + stamp)
			if e.Timed {
				mins := e.Runtime
				if mins <= 0 {
					mins = 30
				}
				line("DTSTART:" + e.Date.UTC().Format("20060102T150405Z"))
				line("DTEND:" + e.Date.UTC().Add(time.Duration(mins)*time.Minute).Format("20060102T150405Z"))
			} else {
				day, _ := time.Parse("2006-01-02", e.AirDay)
				line("DTSTART;VALUE=DATE:" + day.Format("20060102"))
				line("DTEND;VALUE=DATE:" + day.AddDate(0, 0, 1).Format("20060102"))
			}
			line("SUMMARY:" + icalText(what))
			if e.Network != "" {
				line("DESCRIPTION:" + icalText("On "+e.Network))
			}
			line("CATEGORIES:" + icalText(dateWords[e.Kind]))
			line("TRANSP:TRANSPARENT")
			line("END:VEVENT")
			continue
		}
		if e.Kind == "album" {
			// An album is an all-day event on its release day.
			what := e.Name + " - " + e.Album
			if e.Year != nil {
				what += fmt.Sprintf(" (%d)", *e.Year)
			}
			line("BEGIN:VEVENT")
			line(fmt.Sprintf("UID:holyarrs-album-%d@holyarrs", e.PartID))
			line("DTSTAMP:" + stamp)
			line("DTSTART;VALUE=DATE:" + e.Date.UTC().Format("20060102"))
			line("DTEND;VALUE=DATE:" + e.Date.UTC().AddDate(0, 0, 1).Format("20060102"))
			line("SUMMARY:" + icalText(what))
			if e.Tracks > 0 {
				line("DESCRIPTION:" + icalText(fmt.Sprintf("%d of %d tracks in the Library", e.Have, e.Tracks)))
			}
			line("CATEGORIES:" + icalText(dateWords[e.Kind]))
			line("TRANSP:TRANSPARENT")
			line("END:VEVENT")
			continue
		}
		name := e.Name
		if e.Year != nil {
			name += fmt.Sprintf(" (%d)", *e.Year)
		}
		line("BEGIN:VEVENT")
		line(fmt.Sprintf("UID:holyarrs-movie-%d-%s@holyarrs", e.TitleID, e.Kind))
		line("DTSTAMP:" + stamp)
		line("DTSTART;VALUE=DATE:" + e.Date.UTC().Format("20060102"))
		line("DTEND;VALUE=DATE:" + e.Date.UTC().AddDate(0, 0, 1).Format("20060102"))
		line("SUMMARY:" + icalText(name+" - "+dateWords[e.Kind]))
		line("CATEGORIES:" + icalText(dateWords[e.Kind]))
		line("TRANSP:TRANSPARENT")
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String()
}

func icalText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "").Replace(s)
}
