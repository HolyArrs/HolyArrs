package sources

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// heldServer answers a show search with search (name and TVDB id, 0 for
// none) and the feed with an episode 2 release, so a feed check judges the
// waiting episode 1 release again.
func heldServer(t *testing.T, search func() (string, int)) string {
	t.Helper()
	one := func(name string, tvdb int) string {
		id := ""
		if tvdb > 0 {
			id = fmt.Sprintf(`<torznab:attr name="tvdbid" value="%d"/>`, tvdb)
		}
		return fmt.Sprintf(`<item><title>%s</title><pubDate>%s</pubDate><enclosure url="http://src/%s.torrent" length="%d" type="application/x-bittorrent"/><torznab:attr name="seeders" value="50"/>%s</item>`,
			name, time.Now().UTC().Format(time.RFC1123Z), name, int64(1200)<<20, id)
	}
	wrap := func(items string) string {
		return `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>` + items + `</channel></rss>`
	}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("t") == "caps":
			_, _ = w.Write([]byte(tvCaps))
		case q.Get("q") != "" || q.Get("season") != "" || q.Get("tvdbid") != "":
			_, _ = w.Write([]byte(wrap(one(search()))))
		default:
			_, _ = w.Write([]byte(wrap(one("Copper.Valley.S01E02.1080p.WEB-DL.H.264-QUILL", 0))))
		}
	})
	return srv.URL + "/api"
}

func waitOver(t *testing.T, f fixture) {
	exec(t, f, `UPDATE slots SET next_try_at = '2020-01-01T00:00:00Z' WHERE state = 'held'`)
	exec(t, f, `UPDATE releases SET hold_until = '2020-01-01T00:00:00Z' WHERE state = 'held'`)
}

// A release taken by the Source's id keeps waiting through a feed check
// and is grabbed when its wait ends.
func TestHeld_TakenByIDSurvivesFeedCheck(t *testing.T) {
	rel := "Copper.Valley.2019.S01E01.1080p.WEB-DL.H.264-OXBOW"
	f := seedShow(t, heldServer(t, func() (string, int) { return rel, 4242 }), nil)
	waitMinutes(t, f.fixture, 60)
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
	if st := f.state(t, "S01E01"); st != mediafiles.SlotHeld {
		t.Fatalf("after search: slot %s, want held", st)
	}
	runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if st, rs := f.state(t, "S01E01"), relByName(t, f.fixture, rel).State; st != mediafiles.SlotHeld || rs != "held" {
		t.Fatalf("after the feed check: slot %s, release %s, want both held", st, rs)
	}
	waitOver(t, f.fixture)
	runKind(t, f.fixture, sr, jobs.KindGrabHeld, nil, nil)
	if st := f.state(t, "S01E01"); st != mediafiles.SlotChosen {
		t.Fatalf("after the wait: slot %s, want chosen", st)
	}
}

// A waiting release that no longer passes when judged again frees its Slot,
// and the next search can take another release.
func TestHeld_FailingReleaseFreesSlot(t *testing.T) {
	for _, c := range []struct {
		why    string
		rel    string
		breaks string // what changes while the release waits
	}{
		{"its year is no longer the show's", "Copper.Valley.2015.S01E01.1080p.WEB-DL.H.264-OXBOW", `UPDATE titles SET year = 2016`},
		{"it has too few seeders now", "Copper.Valley.S01E01.1080p.WEB-DL.H.264-OXBOW", `UPDATE sources SET min_seeders = 500`},
	} {
		t.Run(c.why, func(t *testing.T) {
			answer := c.rel
			f := seedShow(t, heldServer(t, func() (string, int) { return answer, 0 }), nil)
			waitMinutes(t, f.fixture, 60)
			sr := &Searcher{Store: f.s}
			runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
			if st := f.state(t, "S01E01"); st != mediafiles.SlotHeld {
				t.Fatalf("after search: slot %s, want held", st)
			}
			exec(t, f.fixture, c.breaks)
			runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
			if st, rs := f.state(t, "S01E01"), relByName(t, f.fixture, c.rel).State; st != mediafiles.SlotWanted || rs == "held" {
				t.Fatalf("after the feed check: slot %s, release %s, want the slot looking again and the release not held", st, rs)
			}
			exec(t, f.fixture, `UPDATE sources SET min_seeders = 0`)
			exec(t, f.fixture, `UPDATE slots SET next_try_at = NULL`)
			answer = "Copper.Valley.S01E01.1080p.WEB-DL.H.264-QUILL"
			runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
			var held *int64
			_ = f.s.Read().QueryRow("SELECT held_release_id FROM slots WHERE id = ?", f.slots["S01E01"]).Scan(&held)
			if st := f.state(t, "S01E01"); st != mediafiles.SlotHeld || held == nil || *held != relByName(t, f.fixture, answer).ID {
				t.Fatalf("after the next search: slot %s, want held on %s", st, answer)
			}
		})
	}
}
