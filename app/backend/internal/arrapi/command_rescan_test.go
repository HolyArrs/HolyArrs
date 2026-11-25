package arrapi

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// A Rescan command queues the job that really rescans the folder, and a
// Rescan with no id rescans the whole library instead of answering done.
func TestCommandRescanQueuesRescan(t *testing.T) {
	r := newPhoneRig(t)
	movie := r.exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, keep_up_to_date)
		VALUES (?, '{}', ?, '{}', 'movie', 'A Movie', 'a movie', 2020, '{}', 'tmdb', 'import', 'active', 1)`, pinStamp, pinStamp)
	show, _ := r.show("Phone Show", "4242", true, 1, func(k int) string { return pinDay(-k) })

	count := func(kind string) int {
		var n int
		if err := r.st.Read().QueryRow(`SELECT count(*) FROM jobs WHERE kind = ?`, kind).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	subject := func(kind string) string {
		var s string
		if err := r.st.Read().QueryRow(`SELECT subject FROM jobs WHERE kind = ? ORDER BY id LIMIT 1`, kind).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// A movie rescan queues rescan_title for that movie, not scan_title.
	st, b := r.call("radarr", "POST", "command", fmt.Sprintf(`{"name":"RescanMovie","movieId":%d}`, movie))
	if st != 201 {
		t.Fatalf("rescan movie: %d %s", st, b)
	}
	if n := count(library.KindRescanTitle); n != 1 {
		t.Fatalf("rescan movie: %d rescan_title jobs", n)
	}
	var sub struct {
		Type string `json:"type"`
		ID   int64  `json:"id"`
	}
	if err := json.Unmarshal([]byte(subject(library.KindRescanTitle)), &sub); err != nil || sub.Type != "title" || sub.ID != movie {
		t.Fatalf("rescan movie: subject = %q (err %v)", subject(library.KindRescanTitle), err)
	}
	if n := count(library.KindScanTitle); n != 0 {
		t.Fatalf("rescan movie: %d scan_title jobs queued", n)
	}

	// A series rescan queues rescan_title for that show.
	st, b = r.call("sonarr", "POST", "command", fmt.Sprintf(`{"name":"RescanSeries","seriesId":%d}`, show))
	if st != 201 {
		t.Fatalf("rescan series: %d %s", st, b)
	}
	if n := count(library.KindRescanTitle); n != 2 {
		t.Fatalf("rescan series: %d rescan_title jobs", n)
	}

	// A rescan with no id rescans the whole library instead of answering done.
	st, b = r.call("radarr", "POST", "command", `{"name":"RescanMovie"}`)
	if st != 201 {
		t.Fatalf("rescan all: %d %s", st, b)
	}
	if n := count(library.KindRescan); n != 1 {
		t.Fatalf("rescan all: %d rescan_library jobs", n)
	}
}
