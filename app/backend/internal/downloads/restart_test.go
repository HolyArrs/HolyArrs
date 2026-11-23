package downloads

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// fakeRun is an engine that names the run of its own process, like aria2.
type fakeRun struct {
	Engine
	session string
	fault   bool
}

func (f *fakeRun) Session(context.Context) (string, error) {
	if f.fault {
		return "", ErrNotFound
	}
	return f.session, nil
}

// restartedAt says when the client started again, and never when it cannot say.
func TestManagerSaysWhenAClientRestarted(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	run := &fakeRun{session: "one"}
	if at := m.restartedAt(ctx, run, 7); !at.IsZero() {
		t.Errorf("the first look at a client is a restart at %v", at)
	}
	if at := m.restartedAt(ctx, run, 7); !at.IsZero() {
		t.Errorf("the same run is a restart at %v", at)
	}
	w.clock = w.clock.Add(time.Minute)
	run.session = "two"
	if at := m.restartedAt(ctx, run, 7); !at.Equal(w.clock) {
		t.Errorf("restart at %v, want %v", at, w.clock)
	}
	// The answer stands for the next syncs too, which are the ones that count a
	// download missing long enough to be gone.
	restarted := w.clock
	w.clock = w.clock.Add(time.Minute)
	if at := m.restartedAt(ctx, run, 7); !at.Equal(restarted) {
		t.Errorf("restart at %v, want %v", at, restarted)
	}
	if at := m.restartedAt(ctx, &fakeRun{session: "x", fault: true}, 8); !at.IsZero() {
		t.Errorf("a client that cannot answer is a restart at %v", at)
	}
	if at := m.restartedAt(ctx, &Sabnzbd{}, 9); !at.IsZero() { // a client that does not name its run
		t.Errorf("a client without a run id is a restart at %v", at)
	}
	// An edit may be another client at another address, so what was seen says nothing.
	m.Forget(7)
	if at := m.restartedAt(ctx, run, 7); !at.IsZero() {
		t.Errorf("after an edit the old run counted: %v", at)
	}
}

// A download a restarted aria2 forgot goes, but its row says why: nothing was
// removed and nothing was deleted.
func TestVanishedAfterRestartNamesTheRestart(t *testing.T) {
	ctx := context.Background()
	w, m := setup(t)
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	dr := domain.Downloader{Name: "Aria2", Adapter: "aria2", Category: "movies", Protocol: "torrent"}
	w.clock = w.clock.Add(5 * time.Minute) // past the grace a sent Download gets

	// A restart before the download was last seen there says nothing about it.
	m.apply(ctx, dr, d, Job{}, false, m.settings(ctx), lastMoved(d).Add(-time.Minute))
	if got := w.download(t, id); got.State != "vanished" || got.StateReason == nil || !strings.Contains(*got.StateReason, "removed outside HolyArrs") {
		t.Fatalf("state %s, reason %v", got.State, got.StateReason)
	}

	w, m = setup(t)
	if id, err = m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	d = w.download(t, id)
	w.clock = w.clock.Add(5 * time.Minute)
	m.apply(ctx, dr, d, Job{}, false, m.settings(ctx), lastMoved(d).Add(time.Minute))
	got := w.download(t, id)
	if got.State != "vanished" || got.StateReason == nil || !strings.Contains(*got.StateReason, "restarted and forgot the download") {
		t.Fatalf("state %s, reason %v", got.State, got.StateReason)
	}
}
