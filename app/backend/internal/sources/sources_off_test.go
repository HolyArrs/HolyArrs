package sources

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// countingSource answers every call with the Dune feed and counts the calls
// that are not the caps read.
func countingSource(t *testing.T, asked *atomic.Int32) string {
	t.Helper()
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(file(t, "caps.xml"))
			return
		}
		asked.Add(1)
		_, _ = w.Write([]byte(duneFeed))
	})
	return srv.URL + "/api"
}

// With the Sources part off ("I add downloads myself"), the automatic and
// scheduled searches end at once without asking any Source, without an
// error, and leave the Slot as it was.
func TestSourcesOffSkipsAutomaticSearchesQuietly(t *testing.T) {
	var asked atomic.Int32
	f := seed(t, countingSource(t, &asked))
	exec(t, f, "UPDATE sources SET use_for = json_set(use_for, '$.feed', json('true'))")
	off := func() bool { return false }
	slot := &domain.Subject{Type: "slot", ID: f.slotID}
	for _, k := range []struct {
		kind string
		subj *domain.Subject
	}{{jobs.KindSearchSlot, slot}, {jobs.KindSearchMissing, nil}, {jobs.KindFeedCheck, nil}, {jobs.KindGrabHeld, nil}} {
		exec(t, f, "DELETE FROM jobs")
		j := runKind(t, f, &Searcher{Store: f.s, SourcesOn: off}, k.kind, k.subj, nil)
		if j.State != "done" || j.Error != nil {
			t.Fatalf("%s: %s %+v", k.kind, j.State, j.Error)
		}
		if j.Result["skipped"] != SkippedSourcesOff {
			t.Fatalf("%s result %+v", k.kind, j.Result)
		}
		var queued int
		_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE id != ?", j.ID).Scan(&queued)
		if queued != 0 {
			t.Fatalf("%s queued %d more Jobs", k.kind, queued)
		}
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("a Source was asked %d times", n)
	}
	if sl := slotState(t, f); sl.State != mediafiles.SlotWanted {
		t.Fatalf("slot %s", sl.State)
	}
	evs, _ := f.s.RecentEvents(context.Background(), 10)
	if len(evs) != 0 {
		t.Fatalf("a skipped search wrote Events: %+v", evs)
	}
	// Nothing scheduled is even queued while the part is off.
	sr := &Searcher{Store: f.s, SourcesOn: off}
	if due, _, err := sr.ProbeFeed(context.Background(), time.Now()); due || err != nil {
		t.Fatalf("feed check probe: %v %v", due, err)
	}
	if due, _, err := sr.ProbeGrabHeld(context.Background(), time.Now()); due || err != nil {
		t.Fatalf("grab held probe: %v %v", due, err)
	}
}

// A search a person asks for still runs with the part off.
func TestSourcesOffStillRunsAPersonsSearch(t *testing.T) {
	var asked atomic.Int32
	f := seed(t, countingSource(t, &asked))
	sr := &Searcher{Store: f.s, SourcesOn: func() bool { return false }}
	j := runKind(t, f, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true})
	if j.State != "done" || j.Result["skipped"] != nil {
		t.Fatalf("job %s %+v", j.State, j.Result)
	}
	if asked.Load() == 0 {
		t.Fatal("the person's search asked no Source")
	}
	var rels int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM releases").Scan(&rels)
	if rels == 0 {
		t.Fatal("the person's search stored no Release")
	}
}
