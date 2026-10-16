package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func addDatedMovie(t *testing.T, s *store.Store, name string, cinema, digital time.Time) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		y := cinema.Year()
		ti := &domain.Title{Kind: "movie", GateDriver: "none", Name: name, SortName: strings.ToLower(name), Year: &y, MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{},
			MusicTypes: []string{}, Artwork: []string{}, CinemaDate: &cinema, DigitalDate: &digital}
		_, err := store.Insert(tx, "titles", ti, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

// route: GET /api/v1/calendar/feed.ics
// route: GET /api/v1/calendar/feed-key
// route: POST /api/v1/calendar/feed-key
// route: DELETE /api/v1/calendar/feed-key
func TestCalendarFeedWithItsOwnKey(t *testing.T) {
	r := newAskRig(t)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	addDatedMovie(t, r.st, "Arrival, Part One", today.AddDate(0, 0, 3), today.AddDate(0, 0, 60))
	feed := r.srv.URL + FeedPath
	// Off until a key is made; no sign-in, session or API key opens it.
	if code, _, _ := get(t, feed+"?key=anything"); code != 401 {
		t.Fatalf("feed off: %d", code)
	}
	if b := r.owner.call("GET", "/api/v1/calendar/feed-key", "", 200); !strings.Contains(string(b), `"set":false`) {
		t.Fatalf("key state %s", b)
	}
	r.ana.call("POST", "/api/v1/calendar/feed-key", "", 403)
	var made struct {
		Key  string `json:"key"`
		Path string `json:"path"`
	}
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/calendar/feed-key", "", 201), &made)
	if len(made.Key) < 40 || !strings.HasSuffix(made.Path, made.Key) {
		t.Fatalf("made %+v", made)
	}
	// Shown once: reading it back says only that it is set, and only its hash is stored.
	if b := r.owner.call("GET", "/api/v1/calendar/feed-key", "", 200); strings.Contains(string(b), made.Key) || !strings.Contains(string(b), `"set":true`) {
		t.Fatalf("key read back %s", b)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM settings WHERE value = ?", made.Key).Scan(&n)
	if n != 0 {
		t.Fatal("the plain key is stored")
	}
	code, body, h := get(t, feed+"?key="+made.Key)
	if code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "text/calendar") {
		t.Fatalf("feed %d %s", code, h)
	}
	for _, want := range []string{"BEGIN:VCALENDAR\r\n", "DTSTART;VALUE=DATE:" + today.AddDate(0, 0, 3).Format("20060102"),
		`SUMMARY:Arrival\, Part One (` + today.AddDate(0, 0, 3).Format("2006") + `) - In cinemas`, "END:VCALENDAR\r\n"} {
		if !strings.Contains(body, want) {
			t.Fatalf("feed lacks %q:\n%s", want, body)
		}
	}
	// The digital date is 60 days out: past the default 28, inside future_days=90.
	if strings.Contains(body, "Digital release") {
		t.Fatal("a date past future_days is in the feed")
	}
	if _, body, _ := get(t, feed+"?key="+made.Key+"&future_days=90&kinds=digital"); !strings.Contains(body, "Digital release") || strings.Contains(body, "In cinemas") {
		t.Fatalf("future_days and kinds: %s", body)
	}
	if code, _, _ := get(t, feed+"?key="+made.Key+"&past_days=999"); code != 400 {
		t.Fatalf("past_days out of range: %d", code)
	}
	// A new key ends the old link; turning the feed off ends every link.
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/calendar/feed-key", "", 201), &struct{}{})
	if code, _, _ := get(t, feed+"?key="+made.Key); code != 401 {
		t.Fatalf("old key still works: %d", code)
	}
	r.owner.call("DELETE", "/api/v1/calendar/feed-key", "", 204)
	if b := r.owner.call("GET", "/api/v1/calendar/feed-key", "", 200); !strings.Contains(string(b), `"set":false`) {
		t.Fatalf("after off %s", b)
	}
}

func TestICalFoldsLongLines(t *testing.T) {
	y := 2026
	long := strings.Repeat("Ünïcode name ", 10)
	out := ICal([]library.CalendarEntry{{TitleID: 7, Name: long, Year: &y, Kind: "physical", Date: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}}, time.Now())
	for _, l := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Fatalf("line of %d octets: %q", len(l), l)
		}
	}
	if !strings.Contains(out, "UID:holyarrs-movie-7-physical@holyarrs") || !strings.Contains(out, "DTEND;VALUE=DATE:20261103") {
		t.Fatalf("event %s", out)
	}
}

// 5.2.7: an episode with an air time is a timed event in UTC for its
// running time; one with only an air day is an all-day event.
func TestICalEpisodes(t *testing.T) {
	se, ep := 1, 2
	at := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	out := ICal([]library.CalendarEntry{
		{TitleID: 3, Name: "Copper Valley", Kind: "episode", ItemID: 41, Season: &se, Episode: &ep, EpisodeName: "The Mill", Date: at, Timed: true, AirDay: "2026-10-11", Runtime: 45, Network: "Lab TV"},
		{TitleID: 3, Name: "Copper Valley", Kind: "episode", ItemID: 42, Season: &se, Episode: &ep, Date: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), AirDay: "2026-10-20"},
	}, time.Now())
	for _, want := range []string{"UID:holyarrs-episode-41@holyarrs", "DTSTART:20261012T020000Z", "DTEND:20261012T024500Z", "SUMMARY:Copper Valley - S01E02 - The Mill",
		"DESCRIPTION:On Lab TV", "UID:holyarrs-episode-42@holyarrs", "DTSTART;VALUE=DATE:20261020", "DTEND;VALUE=DATE:20261021", "CATEGORIES:Episode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("feed lacks %q:\n%s", want, out)
		}
	}
}
