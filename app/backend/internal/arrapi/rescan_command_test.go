package arrapi

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// jobCount counts queued Jobs of one kind, with or without a title subject.
func (r *pinRig) jobCount(kind string, titled bool) int {
	r.t.Helper()
	q := "SELECT count(*) FROM jobs WHERE kind = ? AND (subject IS NULL OR subject = 'null')"
	if titled {
		q = "SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.type') = 'title'"
	}
	var n int
	if err := r.st.Read().QueryRow(q, kind).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

// A Rescan command with a title rescans that title; without one it
// rescans the whole Library, as the old apps do.
func TestRescanCommandQueuesRescanJobs(t *testing.T) {
	r := newPinRig(t)
	show, _ := r.show("Rescan Show", "808", true, 1, func(int) string { return "" })
	movie := r.movie("Rescan Movie", "809", 1)
	deps := Deps{Store: r.st, Queue: jobs.New(r.st, jobs.Options{}), Now: func() time.Time { return pinClock }}
	son, rad := Sonarr(deps), Radarr(deps)
	post := func(body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		h, path := son, "/sonarr/api/v3/command"
		if strings.Contains(body, "Movie") {
			h, path = rad, "/radarr/api/v3/command"
		}
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		if rec.Code != 201 {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	post(fmt.Sprintf(`{"name":"RescanSeries","seriesId":%d}`, show))
	post(fmt.Sprintf(`{"name":"RescanMovie","movieId":%d}`, movie))
	if n := r.jobCount("rescan_title", true); n != 2 {
		t.Fatalf("rescan_title jobs: %d, want 2", n)
	}
	post(`{"name":"RescanSeries"}`)
	if n := r.jobCount("rescan_library", false); n != 1 {
		t.Fatalf("rescan_library jobs: %d, want 1", n)
	}
}
