package importer

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// strandedCase is one finished download whose Slots were woken for an import that then never ran.
type strandedCase struct {
	name  string
	s     *database.Store
	dlID  int64
	slots []int64
}

func strandedCases(t *testing.T) []strandedCase {
	t.Helper()
	m := setup(t)
	sh := showSetup(t, []int{1, 2}, ep(1), ep(2))
	al := albumSetup(t, 1, "01 - Dawn.flac", "02 - Glass River.flac", "03 - Night Train.flac")
	cases := []strandedCase{
		{"movie", m.s, m.dlID, []int64{m.slotID}},
		{"show", sh.s, sh.dlID, []int64{sh.slots[1], sh.slots[2]}},
		{"album", al.s, al.dlID, al.slots},
	}
	for _, c := range cases {
		if err := execSQL(c.s, "UPDATE slots SET state = 'importing', last_reason = 'importing', active_download_id = ?", c.dlID); err != nil {
			t.Fatal(err)
		}
	}
	return cases
}

func (c strandedCase) states(t *testing.T) []mediafiles.Slot {
	t.Helper()
	var out []mediafiles.Slot
	for _, id := range c.slots {
		var sl mediafiles.Slot
		if err := database.Get(context.Background(), c.s.Read(), "slots", id, &sl); err != nil {
			t.Fatal(err)
		}
		out = append(out, sl)
	}
	return out
}

// A Slot left importing on a finished download with no import job (the app stopped between the wake and the queue) is imported by the sweeper.
func TestSweeperImportsASlotLeftImporting(t *testing.T) {
	for _, c := range strandedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			im := &Importer{Store: c.s}
			q := jobs.New(c.s, jobs.Options{})
			if _, _, err := q.Enqueue(ctx, KindImport, &domain.Subject{Type: "download", ID: c.dlID}, nil, "import:held", domain.System); err != nil {
				t.Fatal(err)
			}
			if n, err := im.RequeueStranded(ctx, func(context.Context, int64) { t.Error("queued beside a waiting import job") }); err != nil || n != 0 {
				t.Fatalf("with an import job waiting: %d %v", n, err)
			}
			if err := c.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("UPDATE jobs SET state = 'cancelled'"); return err }); err != nil {
				t.Fatal(err)
			}
			n, err := im.RequeueStranded(ctx, func(ctx context.Context, id int64) {
				if _, err := im.Import(ctx, id, false, domain.System); err != nil {
					t.Errorf("import: %v", err)
				}
			})
			if err != nil || n != 1 {
				t.Fatalf("requeued %d %v", n, err)
			}
			for _, sl := range c.states(t) {
				if sl.FileID == nil || sl.State == mediafiles.SlotImporting {
					t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
				}
			}
		})
	}
}

// A Slot left importing whose download HolyArrs no longer has goes back to a card that says so.
func TestSweeperReturnsASlotWhoseDownloadIsGone(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing', last_reason = 'importing', active_download_id = NULL"); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "DELETE FROM downloads"); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	if _, err := im.RequeueStranded(ctx, func(context.Context, int64) { t.Error("queued an import of a download that is gone") }); err != nil {
		t.Fatal(err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != downloadGoneWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// An import job that fails on its last attempt leaves a card with a reason, never a Slot that stays importing.
func TestImportJobThatGivesUpLeavesACard(t *testing.T) {
	for _, c := range strandedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			im := &Importer{Store: c.s}
			// The import cannot read where the files go, on every attempt.
			if err := execSQL(c.s, "ALTER TABLE library_folders RENAME TO library_folders_away"); err != nil {
				t.Fatal(err)
			}
			run := func(attempt int) error {
				return im.HandleJob(ctx, &jobs.Run{Job: jobs.Job{Attempt: attempt, MaxAttempts: 5, Subject: &domain.Subject{Type: "download", ID: c.dlID}, Params: map[string]any{}}})
			}
			for attempt := 1; attempt <= 4; attempt++ {
				if err := run(attempt); err == nil {
					t.Fatal("the import did not fail")
				}
				for _, sl := range c.states(t) {
					if sl.State != mediafiles.SlotImporting {
						t.Fatalf("attempt %d: slot %s", attempt, sl.State)
					}
				}
			}
			if err := run(5); err == nil {
				t.Fatal("the import did not fail")
			}
			for _, sl := range c.states(t) {
				if sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != couldNotPlace(wentWrongWhy) {
					t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
				}
			}
		})
	}
}

// A dry run that fails changes no Slot.
func TestDryRunJobThatFailsLeavesTheSlot(t *testing.T) {
	w := setup(t)
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing', last_reason = 'importing'"); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE downloads SET state = 'downloading'"); err != nil {
		t.Fatal(err)
	}
	err := (&Importer{Store: w.s}).HandleJob(context.Background(), &jobs.Run{Job: jobs.Job{Attempt: 5, MaxAttempts: 5, Subject: &domain.Subject{Type: "download", ID: w.dlID}, Params: map[string]any{"dry_run": true}}})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotImporting {
		t.Fatalf("slot %s", sl.State)
	}
}

// filesFault makes every import fail after its files are placed, as a locked database does; heal takes the fault away.
func filesFault(t *testing.T, s *database.Store) (heal func()) {
	t.Helper()
	if err := execSQL(s, "CREATE TRIGGER files_fault BEFORE INSERT ON files BEGIN SELECT RAISE(ABORT, 'database is locked'); END"); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := execSQL(s, "DROP TRIGGER files_fault"); err != nil {
			t.Fatal(err)
		}
	}
}

// giveUp runs the import job of c until its last attempt has failed.
func (c strandedCase) giveUp(t *testing.T, im *Importer) {
	t.Helper()
	for attempt := 1; attempt <= 5; attempt++ {
		err := im.HandleJob(context.Background(), &jobs.Run{Job: jobs.Job{Attempt: attempt, MaxAttempts: 5, Subject: &domain.Subject{Type: "download", ID: c.dlID}, Params: map[string]any{}}})
		if err == nil {
			t.Fatal("the import did not fail")
		}
	}
	for _, sl := range c.states(t) {
		if sl.State != mediafiles.SlotNeedsADecision || !CanImportAgain(ptrText(sl.LastReason)) {
			t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
		}
	}
}

func (c strandedCase) wantImported(t *testing.T) {
	t.Helper()
	for _, sl := range c.states(t) {
		if sl.FileID == nil || sl.State == mediafiles.SlotImporting || sl.State == mediafiles.SlotNeedsADecision {
			t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
		}
	}
}

// A show or album import that gave up with its files already placed heals on a later sweep once the fault is gone, as it always did; a movie waits for its person.
func TestSweeperHealsAnImportThatGaveUp(t *testing.T) {
	for _, c := range strandedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			im := &Importer{Store: c.s}
			heal := filesFault(t, c.s)
			c.giveUp(t, im)
			heal()
			n, err := im.RequeueStranded(ctx, func(ctx context.Context, id int64) {
				if _, err := im.Import(ctx, id, false, domain.System); err != nil {
					t.Errorf("import: %v", err)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if c.name == "movie" {
				if sl := c.states(t)[0]; n != 0 || sl.State != mediafiles.SlotNeedsADecision {
					t.Fatalf("movie: requeued %d, slot %s", n, sl.State)
				}
				return
			}
			if n != 1 {
				t.Fatalf("requeued %d", n)
			}
			c.wantImported(t)
		})
	}
}

// The card of an import that gave up offers Import again: the woken Slots are imported once the fault is gone.
func TestImportAgainHealsAnImportThatGaveUp(t *testing.T) {
	for _, c := range strandedCases(t) {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			im := &Importer{Store: c.s}
			heal := filesFault(t, c.s)
			c.giveUp(t, im)
			heal()
			// What Import again does: the Slots go back to importing, keeping their reason.
			if err := execSQL(c.s, "UPDATE slots SET state = 'importing' WHERE state = 'needs_a_decision'"); err != nil {
				t.Fatal(err)
			}
			if _, err := im.Import(ctx, c.dlID, false, domain.System); err != nil {
				t.Fatal(err)
			}
			c.wantImported(t)
		})
	}
}

// A Slot left importing on a download that failed or was cleaned meanwhile goes back to a card that says the download is gone.
func TestSweeperReturnsASlotWhoseDownloadEnded(t *testing.T) {
	for _, state := range []string{"failed", "vanished", "cleaned"} {
		w := setup(t)
		if err := execSQL(w.s, "UPDATE slots SET state = 'importing', last_reason = 'importing', active_download_id = ?", w.dlID); err != nil {
			t.Fatal(err)
		}
		if err := execSQL(w.s, "UPDATE downloads SET state = ?", state); err != nil {
			t.Fatal(err)
		}
		im := &Importer{Store: w.s}
		if _, err := im.RequeueStranded(context.Background(), func(context.Context, int64) { t.Error("queued an import of a download that ended") }); err != nil {
			t.Fatal(err)
		}
		if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != downloadGoneWhy {
			t.Fatalf("%s: slot %s reason %q", state, sl.State, ptrText(sl.LastReason))
		}
	}
}

// A Slot importing while its download is still downloading (a season pack fetched a few at a time) is left alone.
func TestSweeperLeavesASlotOfADownloadStillRunning(t *testing.T) {
	w := setup(t)
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing', last_reason = 'importing', active_download_id = ?", w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE downloads SET state = 'downloading'"); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).RequeueStranded(context.Background(), func(context.Context, int64) {}); err != nil {
		t.Fatal(err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotImporting {
		t.Fatalf("slot %s", sl.State)
	}
}

// An import that stops because the app is stopping is not a last attempt that failed: the Slot stays for the next start.
func TestImportJobCancelledLeavesTheSlotImporting(t *testing.T) {
	w := setup(t)
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing', last_reason = 'importing', active_download_id = ?", w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "ALTER TABLE library_folders RENAME TO library_folders_away"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := (&Importer{Store: w.s}).HandleJob(ctx, &jobs.Run{Job: jobs.Job{Attempt: 5, MaxAttempts: 5, Subject: &domain.Subject{Type: "download", ID: w.dlID}, Params: map[string]any{}}})
	if err == nil {
		t.Fatal("no error")
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotImporting {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// A Slot left importing on a re-grab that failed goes back to a card while an older download of the same Slot still seeds.
func TestSweeperReturnsASlotWhoseRegrabFailed(t *testing.T) {
	w := setup(t)
	again := copyDownload(t, w.s, w.dlID, "failed")
	if err := execSQL(w.s, "UPDATE downloads SET state = 'seeding' WHERE id = ?", w.dlID); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET state = 'importing', last_reason = 'importing', active_download_id = ?", again); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).RequeueStranded(context.Background(), func(context.Context, int64) { t.Error("queued an import") }); err != nil {
		t.Fatal(err)
	}
	if sl := slot308(t, w); sl.State != mediafiles.SlotNeedsADecision || ptrText(sl.LastReason) != downloadGoneWhy {
		t.Fatalf("slot %s reason %q", sl.State, ptrText(sl.LastReason))
	}
}

// A gave-up show or album card whose Slots now wait for another download gets no import queued.
func TestSweeperSkipsAGaveUpCardOfAnotherDownload(t *testing.T) {
	for _, c := range strandedCases(t) {
		if c.name == "movie" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			im := &Importer{Store: c.s}
			heal := filesFault(t, c.s)
			c.giveUp(t, im)
			heal()
			if err := execSQL(c.s, "UPDATE slots SET active_download_id = ?", copyDownload(t, c.s, c.dlID, "failed")); err != nil {
				t.Fatal(err)
			}
			n, err := im.RequeueStranded(context.Background(), func(context.Context, int64) { t.Error("queued an import") })
			if err != nil || n != 0 {
				t.Fatalf("requeued %d, err %v", n, err)
			}
		})
	}
}

// copyDownload adds a download of the same Slots as id, under another hash, in state.
func copyDownload(t *testing.T, s *database.Store, id int64, state string) int64 {
	t.Helper()
	var cols, sel []string
	rows, err := s.Read().Query("SELECT name FROM pragma_table_info('downloads') WHERE name <> 'id'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		cols = append(cols, c)
		switch c {
		case "external_id":
			c = "external_id || 'x'"
		case "state":
			c = "'" + state + "'"
		}
		sel = append(sel, c)
	}
	rows.Close()
	if err := execSQL(s, "INSERT INTO downloads ("+strings.Join(cols, ",")+") SELECT "+strings.Join(sel, ",")+" FROM downloads WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := s.Read().QueryRow("SELECT max(id) FROM downloads").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
