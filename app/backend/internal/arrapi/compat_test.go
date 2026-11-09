package arrapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

func TestInternalErrorHidesDetail(t *testing.T) {
	w := httptest.NewRecorder()
	internalError(w, errors.New("SQL logic error /config/x.db"))
	if w.Code != 500 || strings.Contains(w.Body.String(), "SQL") || strings.Contains(w.Body.String(), "/config") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	internalError(w, fmt.Errorf("title: %w", database.ErrNotFound))
	if w.Code != 404 {
		t.Fatalf("not found: got %d", w.Code)
	}
}

// The answers use the words and formats the old apps use.
func TestFormats_ManualWords(t *testing.T) {
	for sec, want := range map[int64]string{0: "00:00:00", 125: "00:02:05", 3600*23 + 59: "23:00:59", 86400 + 3723: "1.01:02:03", -5: "00:00:00"} {
		if got := timeSpan(sec); got != want {
			t.Errorf("timeSpan(%d) = %q, want %q", sec, got, want)
		}
	}
	if got := words("MissingEpisodeSearch"); got != "Missing Episode Search" {
		t.Errorf("words: %q", got)
	}
	b, _ := json.Marshal(utc(time.Date(2026, 11, 3, 4, 5, 6, 789, time.FixedZone("x", 3600))))
	if string(b) != `"2026-11-03T03:05:06Z"` {
		t.Errorf("date: %s", b)
	}
	if b, _ := json.Marshal(zeroDate); string(b) != `"0001-01-01T00:00:00Z"` {
		t.Errorf("zero date: %s", b)
	}
}

// Anime shows map to absolute numbers and daily shows to air dates, both
// ways.
func TestShowTypes_RoundTrip(t *testing.T) {
	for typ, order := range map[string]string{"anime": library.OrderAbsolute, "Daily": library.OrderDaily, "standard": library.OrderAired, "": library.OrderAired} {
		if got := episodeOrder(typ); got != order {
			t.Errorf("episodeOrder(%q) = %q, want %q", typ, got, order)
		}
	}
	for _, o := range []string{library.OrderAbsolute, library.OrderDaily, library.OrderAired} {
		back := seriesType(&o)
		if episodeOrder(back) != o {
			t.Errorf("%s -> %s does not come back", o, back)
		}
	}
	if seriesType(nil) != "standard" {
		t.Error("no order is standard")
	}
}

// The minimum availability stages map to Release gates and back.
func TestStages_RoundTrip(t *testing.T) {
	for stage, gate := range map[string]string{"announced": library.GateAnnounced, "tba": library.GateAnnounced, "inCinemas": library.GateInCinemas,
		"INCINEMAS": library.GateInCinemas, "released": library.GateReleased, "": ""} {
		if got := gateOf(stage); got != gate {
			t.Errorf("gateOf(%q) = %q, want %q", stage, got, gate)
		}
	}
	for _, g := range []string{library.GateAnnounced, library.GateInCinemas, library.GateReleased} {
		if gateOf(availability(g)) != g {
			t.Errorf("%s does not come back", g)
		}
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past, ahead := now.AddDate(0, -1, 0), now.AddDate(0, 1, 0)
	for _, c := range []struct {
		cinema, digital *time.Time
		want            string
	}{{nil, nil, "tba"}, {&ahead, nil, "announced"}, {&past, nil, "inCinemas"}, {&past, &past, "released"}, {&past, &ahead, "inCinemas"}} {
		if got := movieStatus(c.cinema, c.digital, nil, now); got != c.want {
			t.Errorf("movieStatus = %q, want %q", got, c.want)
		}
	}
}
