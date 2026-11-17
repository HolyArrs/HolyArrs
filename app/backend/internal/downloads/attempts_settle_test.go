package downloads

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	old, flags := log.Writer(), log.Flags()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(old); log.SetFlags(flags) })
	return &b
}

// A send NZBGet never got (its reply lost, then no job in its queue or history)
// is settled after the grace: the record closes, the Slot goes back to wanted
// with the reason, and the log says so. Before the grace it waits.
func TestUnknownSendNeverArrivedGoesBackToWanted(t *testing.T) {
	w, m, d := journalWorld(t)
	logs := captureLog(t)
	ctx := context.Background()
	e := &attemptEngine{add: func(r AddRequest) (string, error) {
		if r.ReconcileOnly {
			return "", errors.Join(ErrUncertain, ErrNotArrived)
		}
		return "", errors.Join(ErrUncertain, errors.New("nzbget append: no answer"))
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "warning") || !strings.Contains(logs.String(), "no answer") {
		t.Fatalf("the unknown send was not logged with the reply: %q", logs.String())
	}
	m.settleAttempts(ctx)
	if got := w.slot(t); got.State != mediafiles.SlotChosen {
		t.Fatalf("settled inside the grace: %s", got.State)
	}
	w.advance(notArrivedGrace + time.Minute)
	m.settleAttempts(ctx)
	got := w.slot(t)
	if got.State != mediafiles.SlotWanted || got.LastReason == nil || !strings.Contains(*got.LastReason, "never arrived") {
		t.Fatalf("slot %s %v %s", got.State, got.LastReason, logs.String())
	}
	a, err := m.Attempts(ctx, d.ID)
	if err != nil || len(a) != 1 || a[0].State != "resolved" {
		t.Fatal(a, err)
	}
	if !strings.Contains(logs.String(), "never arrived") {
		t.Fatalf("settling was not logged: %q", logs.String())
	}
}

// A late job found by its marker after the grace becomes the Download.
func TestUnknownSendFoundLaterBecomesTheDownload(t *testing.T) {
	w, m, _ := journalWorld(t)
	ctx := context.Background()
	e := &attemptEngine{add: func(r AddRequest) (string, error) {
		if r.ReconcileOnly {
			return r.AttemptMarker, nil
		}
		return "", errors.Join(ErrUncertain, errors.New("timed out"))
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
	w.advance(notArrivedGrace + time.Minute)
	m.settleAttempts(ctx)
	if got := w.slot(t); got.State != mediafiles.SlotDownloading {
		t.Fatalf("slot %s", got.State)
	}
}

// A Downloader that does not answer the look-up keeps the record for later.
func TestUnknownSendKeptWhileTheDownloaderIsSilent(t *testing.T) {
	w, m, _ := journalWorld(t)
	ctx := context.Background()
	e := &attemptEngine{add: func(AddRequest) (string, error) { return "", errors.Join(ErrUncertain, errDownloaderUnavailable) }}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	_, _ = m.Grab(ctx, w.slotID, w.relID, domain.System)
	w.advance(notArrivedGrace + time.Minute)
	m.settleAttempts(ctx)
	if got := w.slot(t); got.State != mediafiles.SlotChosen {
		t.Fatalf("slot %s", got.State)
	}
}

// A definite refusal is logged with the Downloader's reply.
func TestRefusedSendIsLogged(t *testing.T) {
	w, m, _ := journalWorld(t)
	logs := captureLog(t)
	e := &attemptEngine{add: func(AddRequest) (string, error) {
		return "", errors.New(`NZBGet did not take the NZB: nzbget append: NZBGet answered "Invalid parameter" (code 2)`)
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err == nil || errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "warning") || !strings.Contains(logs.String(), "Invalid parameter") {
		t.Fatalf("log: %q", logs.String())
	}
}
