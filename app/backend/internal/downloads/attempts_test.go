package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

type attemptEngine struct {
	Engine
	mu       sync.Mutex
	requests []AddRequest
	add      func(AddRequest) (string, error)
}

func (e *attemptEngine) Add(_ context.Context, r AddRequest) (string, error) {
	e.mu.Lock()
	e.requests = append(e.requests, r)
	e.mu.Unlock()
	return e.add(r)
}

func (e *attemptEngine) Get(context.Context, string) (Job, error) {
	return Job{}, ErrNotFound
}

func TestNZBGetJournalSurvivesRestartAndPinsOriginal(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	e := &attemptEngine{}
	visible := false
	e.add = func(r AddRequest) (string, error) {
		if r.AttemptMarker == "" {
			t.Fatal("append had no persisted marker")
		}
		var count int
		if err := w.s.Read().QueryRow("SELECT count(*) FROM downloader_attempts WHERE marker = ? AND state = 'pending'", r.AttemptMarker).Scan(&count); err != nil || count != 1 {
			t.Fatal("intent was not durable before Add", err)
		}
		if !visible {
			return "", ErrUncertain
		}
		if !r.ReconcileOnly {
			t.Fatal("visible late acceptance caused append")
		}
		return r.AttemptMarker, nil
	}
	fallback := &attemptEngine{add: func(AddRequest) (string, error) { t.Fatal("unknown outcome failed over"); return "", nil }}
	w.addDownloader(t, &domain.Downloader{Name: "Fallback", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://fallback", Priority: 2})
	m.Engine = func(got domain.Downloader) (Engine, error) {
		if got.ID == d.ID {
			return e, nil
		}
		return fallback, nil
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.Is(err, ErrUncertain) {
		t.Fatal(err)
	}
	a, err := m.Attempts(ctx, d.ID)
	if err != nil || len(a) != 1 || a[0].State != "pending" {
		t.Fatal(a, err)
	}
	b, _ := json.Marshal(a)
	if strings.Contains(string(b), "nzbget-") || strings.Contains(string(b), "marker") || strings.Contains(string(b), "http") {
		t.Fatal("private attempt details exposed")
	}
	dir := w.s.Dir()
	if err := w.s.Close(); err != nil {
		t.Fatal(err)
	}
	w.s, err = database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.s.Close() })
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET state = 'disabled', backoff_until = ? WHERE id = ?", database.FormatTime(time.Now().Add(time.Hour)), d.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m = &Manager{Store: w.s, Engine: m.Engine, Now: w.now, Sleep: func(context.Context, time.Duration) error {
		t.Fatal("reconciliation reserved send spacing")
		return nil
	}, Link: func(context.Context, domain.Release) (string, error) {
		t.Fatal("reconciliation fetched a new Source link")
		return "", nil
	}}
	for range 2 {
		if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.Is(err, ErrUncertain) {
			t.Fatal(err)
		}
	}
	visible = true
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	for i, r := range e.requests {
		if r.AttemptMarker != e.requests[0].AttemptMarker || r.ReconcileOnly != (i > 0) {
			t.Fatal("identity or append permission changed")
		}
		if i > 0 && (r.URL != "" || r.AllowLocal || r.OperationKey != "") {
			t.Fatal("reconciliation received payload fetching inputs")
		}
	}
	a, err = m.Attempts(ctx, d.ID)
	if err != nil || len(a) != 1 || a[0].State != "resolved" {
		t.Fatal(a, err)
	}
	if got := w.slot(t); got.State != mediafiles.SlotDownloading {
		t.Fatal(got.State)
	}
}

func TestNZBGetJournalAbandonedPairAllowsDifferentRelease(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	a, err := m.claimAttempt(ctx, d, w.relID, w.slotID)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AbandonAttempt(ctx, d.ID, a.ID, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotNeedsADecision || sl.ChosenReleaseID != nil {
		t.Fatal("abandon did not release the choice for a person's decision", sl)
	}
	if _, err := (&sources.Searcher{Store: w.s}).Choose(ctx, w.relID, w.slotID, true, domain.System); !errors.Is(err, sources.ErrBusy) {
		t.Fatal("abandoned Release could be chosen again", err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotNeedsADecision || sl.ChosenReleaseID != nil {
		t.Fatal("refused choice stranded the Slot", sl)
	}
	var rel domain.Release
	if err := database.Get(ctx, w.s.Read(), "releases", w.relID, &rel); err != nil {
		t.Fatal(err)
	}
	rel.ID, rel.Name, rel.LinkRef = 0, "Other.Release", "http://fake-indexer/other.nzb"
	if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "releases", &rel, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	if _, err := (&sources.Searcher{Store: w.s}).Choose(ctx, rel.ID, w.slotID, false, domain.System); err != nil {
		t.Fatal("could not choose a different Release after abandonment", err)
	}
	// An old recovery action repeated later must not disturb the new choice.
	if err := m.AbandonAttempt(ctx, d.ID, a.ID, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotChosen || sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != rel.ID {
		t.Fatal("repeated abandon changed the newer choice", sl)
	}
	e := &attemptEngine{add: func(r AddRequest) (string, error) {
		if r.ReconcileOnly {
			t.Fatal("different Release used abandoned attempt")
		}
		return r.AttemptMarker, nil
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	if _, err := m.Grab(ctx, w.slotID, rel.ID, domain.System); err != nil {
		t.Fatal(err)
	}
	got, err := m.Attempts(ctx, d.ID)
	if err != nil || len(got) != 2 || got[0].State != "resolved" || got[1].State != "abandoned" {
		t.Fatal(got, err)
	}
}

func TestOptionalGrabRejectsStaleReleaseChoice(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	if _, err := m.claimAttempt(ctx, d, w.relID, w.slotID); err != nil {
		t.Fatal(err)
	}
	var other domain.Release
	if err := database.Get(ctx, w.s.Read(), "releases", w.relID, &other); err != nil {
		t.Fatal(err)
	}
	other.ID, other.Name, other.LinkRef = 0, "Stale.Job.Release", "http://fake-indexer/stale.nzb"
	if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "releases", &other, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	m.Engine = func(domain.Downloader) (Engine, error) { t.Fatal("stale grab reached Downloader"); return nil, nil }
	if _, err := m.Grab(ctx, w.slotID, other.ID, domain.System); err == nil {
		t.Fatal("grab accepted a Release the Slot did not choose")
	}
	if _, err := m.claimAttempt(ctx, d, other.ID, w.slotID); err == nil {
		t.Fatal("claim accepted a stale choice")
	}
	if a, err := m.Attempts(ctx, d.ID); err != nil || len(a) != 1 || a[0].ReleaseID != w.relID || a[0].State != "pending" {
		t.Fatal("stale grab changed the original attempt", a, err)
	}
	if sl := w.slot(t); sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != w.relID {
		t.Fatal("stale grab changed the choice", sl)
	}
}

func TestOptionalJournalAbandonOnlyReleasesCurrentCoveredChoices(t *testing.T) {
	for _, change := range []string{"none", "downloading", "importing", "different_choice", "slot_recreated", "version_recreated", "release_recreated", "not_covered"} {
		t.Run(change, func(t *testing.T) {
			w, m, d := journalWorld(t)
			ctx := context.Background()
			var second mediafiles.Slot
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				if err := database.Get(ctx, tx, "slots", w.slotID, &second); err != nil {
					return err
				}
				var item mediafiles.Item
				if err := database.Get(ctx, tx, "items", second.ItemID, &item); err != nil {
					return err
				}
				item.ID = 0
				if _, err := database.Insert(tx, "items", &item, domain.System); err != nil {
					return err
				}
				second.ID, second.ItemID = 0, item.ID
				if _, err := database.Insert(tx, "slots", &second, domain.System); err != nil {
					return err
				}
				if change == "not_covered" {
					return nil
				}
				_, err := tx.Exec("UPDATE releases SET verdict = json_set(verdict, '$.pick_slot_ids', json(?)) WHERE id = ?", jsonIDs([]int64{w.slotID, second.ID}), w.relID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			a, err := m.claimAttempt(ctx, d, w.relID, w.slotID)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				var err error
				switch change {
				case "downloading", "importing":
					_, err = tx.Exec("UPDATE slots SET state = ? WHERE id = ?", change, second.ID)
				case "different_choice":
					_, err = tx.Exec("UPDATE slots SET chosen_release_id = NULL WHERE id = ?", second.ID)
				case "slot_recreated":
					_, err = tx.Exec("UPDATE slots SET created_at = ? WHERE id = ?", database.FormatTime(second.CreatedAt.Add(time.Hour)), second.ID)
				case "version_recreated":
					_, err = tx.Exec("UPDATE versions SET created_at = ? WHERE id = ?", database.FormatTime(time.Now().Add(time.Hour)), second.VersionID)
				case "release_recreated":
					_, err = tx.Exec("UPDATE releases SET created_at = ? WHERE id = ?", database.FormatTime(time.Now().Add(time.Hour)), w.relID)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var before mediafiles.Slot
			if err := database.Get(ctx, w.s.Read(), "slots", second.ID, &before); err != nil {
				t.Fatal(err)
			}
			if err := m.AbandonAttempt(ctx, d.ID, a.ID, domain.System); err != nil {
				t.Fatal(err)
			}
			var after mediafiles.Slot
			if err := database.Get(ctx, w.s.Read(), "slots", second.ID, &after); err != nil {
				t.Fatal(err)
			}
			if change == "none" {
				if after.State != mediafiles.SlotNeedsADecision || after.ChosenReleaseID != nil {
					t.Fatal("covered choice was not released", after)
				}
			} else if after.State != before.State || !after.UpdatedAt.Equal(before.UpdatedAt) ||
				(after.ChosenReleaseID == nil) != (before.ChosenReleaseID == nil) ||
				after.ChosenReleaseID != nil && *after.ChosenReleaseID != *before.ChosenReleaseID {
				t.Fatal("abandon changed another or newer choice", change, before, after)
			}
			if sl := w.slot(t); change == "version_recreated" || change == "release_recreated" {
				if sl.State != mediafiles.SlotChosen {
					t.Fatal("old attempt changed a replacement identity", sl)
				}
			} else if sl.State != mediafiles.SlotNeedsADecision || sl.ChosenReleaseID != nil {
				t.Fatal("original current choice was not released", sl)
			}
		})
	}
}

func TestOptionalGrabChoiceChangedDuringSendKeepsAttemptPending(t *testing.T) {
	for _, change := range []string{"choice", "release_recreated", "version_recreated"} {
		t.Run(change, func(t *testing.T) {
			w, m, d := journalWorld(t)
			ctx := context.Background()
			e := &attemptEngine{add: func(r AddRequest) (string, error) {
				if err := w.s.Write(ctx, func(tx *sql.Tx) error {
					var err error
					switch change {
					case "choice":
						_, err = tx.Exec("UPDATE slots SET chosen_release_id = NULL WHERE id = ?", w.slotID)
					case "release_recreated":
						_, err = tx.Exec("UPDATE releases SET created_at = ? WHERE id = ?", database.FormatTime(time.Now().Add(time.Hour)), w.relID)
					case "version_recreated":
						_, err = tx.Exec("UPDATE versions SET created_at = ? WHERE id = (SELECT version_id FROM slots WHERE id = ?)", database.FormatTime(time.Now().Add(time.Hour)), w.slotID)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
				return r.AttemptMarker, nil
			}}
			m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
			if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err == nil {
				t.Fatal("changed identity or choice accepted a late send result")
			}
			var count int
			if err := w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&count); err != nil || count != 0 {
				t.Fatal("changed identity or choice wrote a Download", count, err)
			}
			if sl := w.slot(t); sl.State != mediafiles.SlotChosen {
				t.Fatal("late result moved a changed choice", sl.State)
			}
			if a, err := m.Attempts(ctx, d.ID); err != nil || len(a) != 1 || a[0].State != "pending" {
				t.Fatal("failed finalization lost the unknown outcome", a, err)
			}
		})
	}
}

func TestNZBGetJournalFailedDownloadWriteKeepsPending(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	e := &attemptEngine{add: func(r AddRequest) (string, error) { return r.AttemptMarker, nil }}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("CREATE TRIGGER reject_test_download BEFORE INSERT ON downloads BEGIN SELECT RAISE(ABORT, 'fixture'); END")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err == nil {
		t.Fatal("write should fail")
	}
	a, _ := m.Attempts(ctx, d.ID)
	if len(a) != 1 || a[0].State != "pending" {
		t.Fatal(a)
	}
	if got := w.slot(t); got.State != mediafiles.SlotChosen {
		t.Fatal(got.State)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DROP TRIGGER reject_test_download"); return err }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(e.requests) != 2 || !e.requests[1].ReconcileOnly || e.requests[0].AttemptMarker != e.requests[1].AttemptMarker {
		t.Fatal("failed transaction allowed replay")
	}
}

func TestNZBGetJournalConcurrentClaimOnlyOneAppendOwner(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	const n = 8
	results := make(chan *sendAttempt, n)
	errs := make(chan error, n)
	for range n {
		go func() { a, err := m.claimAttempt(ctx, d, w.relID, w.slotID); results <- a; errs <- err }()
	}
	created := 0
	marker := ""
	for range n {
		a := <-results
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if a == nil {
			t.Fatal("missing intent")
		}
		if a.created {
			created++
		}
		if marker != "" && marker != a.marker {
			t.Fatal("different claims")
		}
		marker = a.marker
	}
	if created != 1 {
		t.Fatal("append owners", created)
	}
}

func TestNZBGetJournalAbandonWinsInflightAndBlocksEdits(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	e := &attemptEngine{add: func(r AddRequest) (string, error) { close(entered); <-release; return r.AttemptMarker, nil }}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	done := make(chan error, 1)
	go func() { _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); done <- err }()
	<-entered
	if _, err := Change(ctx, w.s, nil, d.ID, Input{Name: "Changed", URL: "http://other"}, domain.System); !errors.Is(err, ErrPendingAttempt) {
		t.Fatal(err)
	}
	if err := Remove(ctx, w.s, d.ID, domain.System); !errors.Is(err, ErrPendingAttempt) {
		t.Fatal(err)
	}
	a, _ := m.Attempts(ctx, d.ID)
	for _, by := range []domain.Actor{{Kind: "user", ID: 7}, {Kind: "user", ID: 8}} {
		if err := m.AbandonAttempt(ctx, d.ID, a[0].ID, by); err != nil {
			t.Fatal(err)
		}
	}
	var actorJSON string
	if err := w.s.Read().QueryRow("SELECT abandoned_by FROM downloader_attempts WHERE id = ?", a[0].ID).Scan(&actorJSON); err != nil {
		t.Fatal(err)
	}
	var by domain.Actor
	if err := json.Unmarshal([]byte(actorJSON), &by); err != nil || by != (domain.Actor{Kind: "user", ID: 7}) {
		t.Fatal("abandon did not retain its first actor", by, err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrAttemptStopped) {
		t.Fatal(err)
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.Is(err, ErrAttemptStopped) {
		t.Fatal(err)
	}
	var count int
	if err := w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&count); err != nil || count != 0 {
		t.Fatal("abandoned send wrote Download", err)
	}
	if err := Remove(ctx, w.s, d.ID, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := w.downloader(t, d.ID); !got.Removed {
		t.Fatal("audit history did not preserve removed Downloader")
	}
	if len(e.requests) != 1 {
		t.Fatal("abandoned pair resent")
	}
}

func TestNZBGetJournalDefiniteRefusalCanTryNext(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	w.addDownloader(t, &domain.Downloader{Name: "Fallback", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://fallback", Priority: 2})
	m.Engine = func(got domain.Downloader) (Engine, error) {
		return &attemptEngine{add: func(r AddRequest) (string, error) {
			if got.ID == d.ID {
				return "", errors.New("invalid NZB document")
			}
			return "fallback-job", nil
		}}, nil
	}
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	a, _ := m.Attempts(ctx, d.ID)
	if len(a) != 1 || a[0].State != "resolved" {
		t.Fatal(a)
	}
}

func TestNZBGetJournalAuditDoesNotBlockLibraryCleanup(t *testing.T) {
	for _, state := range []string{"resolved", "abandoned"} {
		t.Run(state, func(t *testing.T) {
			w, m, d := journalWorld(t)
			ctx := context.Background()
			a, err := m.claimAttempt(ctx, d, w.relID, w.slotID)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				if _, err := tx.Exec("UPDATE downloader_attempts SET state = ? WHERE id = ?", state, a.ID); err != nil {
					return err
				}
				_, err := tx.Exec("DELETE FROM items WHERE id = (SELECT item_id FROM slots WHERE id = ?)", w.slotID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			got, err := m.Attempts(ctx, d.ID)
			if err != nil || len(got) != 1 || got[0].SlotID != w.slotID {
				t.Fatal("audit lost", got, err)
			}
			var count int
			if err := w.s.Read().QueryRow("SELECT count(*) FROM slots WHERE id = ?", w.slotID).Scan(&count); err != nil || count != 0 {
				t.Fatal("Slot not removed", err)
			}
		})
	}
}

func TestOptionalJournalDoesNotBindReusedIDs(t *testing.T) {
	for _, state := range []string{"pending", "abandoned"} {
		t.Run(state, func(t *testing.T) {
			w, m, d := journalWorld(t)
			ctx := context.Background()
			old, err := m.claimAttempt(ctx, d, w.relID, w.slotID)
			if err != nil {
				t.Fatal(err)
			}
			var rel domain.Release
			var sl mediafiles.Slot
			if err := database.Get(ctx, w.s.Read(), "releases", w.relID, &rel); err != nil {
				t.Fatal(err)
			}
			if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
				t.Fatal(err)
			}
			if err := w.s.Write(ctx, func(tx *sql.Tx) error {
				if _, err := tx.Exec("UPDATE downloader_attempts SET state = ? WHERE id = ?", state, old.ID); err != nil {
					return err
				}
				if _, err := tx.Exec("DELETE FROM releases WHERE id = ?", rel.ID); err != nil {
					return err
				}
				if _, err := tx.Exec("DELETE FROM slots WHERE id = ?", sl.ID); err != nil {
					return err
				}
				// Recreate both ids on purpose, as SQLite can do without AUTOINCREMENT.
				sl.ChosenReleaseID = nil
				if _, err := database.Insert(tx, "slots", &sl, domain.System); err != nil {
					return err
				}
				if _, err := database.Insert(tx, "releases", &rel, domain.System); err != nil {
					return err
				}
				_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rel.ID, sl.ID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if rel.ID != w.relID || sl.ID != w.slotID {
				t.Fatal("fixture did not reuse ids")
			}
			if got, err := unfinishedAttempt(ctx, w.s.Read(), rel.ID, sl.ID); err != nil || got != nil {
				t.Fatal("old semantic pair was rebound", got, err)
			}
			fresh, err := m.claimAttempt(ctx, d, rel.ID, sl.ID)
			if err != nil || !fresh.created || fresh.marker == old.marker {
				t.Fatal("new pair did not get its own intent", err)
			}
		})
	}
}

func TestOptionalTorrentJournalPinsUnknownOutcomeAcrossRestart(t *testing.T) {
	for _, kind := range []string{"transmission", "deluge", "rtorrent"} {
		t.Run(kind, func(t *testing.T) {
			w, m := setup(t)
			ctx := context.Background()
			if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("UPDATE downloaders SET adapter = ?", kind); return err }); err != nil {
				t.Fatal(err)
			}
			d := w.downloader(t, 1)
			e := &attemptEngine{add: func(r AddRequest) (string, error) {
				if r.ReconcileOnly {
					if r.InfoHash != hash || r.URL != "" {
						t.Fatal("recovery did not use the saved hash only")
					}
					return r.InfoHash, nil
				}
				return "", errors.Join(ErrUncertain, errDownloaderUnavailable)
			}}
			fallback := w.addDownloader(t, &domain.Downloader{Name: "Other torrent", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://fallback", Priority: 2})
			m.Engine = func(got domain.Downloader) (Engine, error) {
				if got.ID == fallback {
					t.Fatal("unknown torrent outcome failed over")
				}
				return e, nil
			}
			if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); !errors.Is(err, ErrUncertain) {
				t.Fatal(err)
			}
			dir := w.s.Dir()
			if err := w.s.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			w.s, err = database.Open(dir, database.Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = w.s.Close() })
			m = &Manager{Store: w.s, Engine: m.Engine, Now: w.now, Link: func(context.Context, domain.Release) (string, error) {
				t.Fatal("recovery fetched Source link")
				return "", nil
			}}
			if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
				t.Fatal(err)
			}
			a, _ := m.Attempts(ctx, d.ID)
			if len(a) != 1 || a[0].State != "resolved" {
				t.Fatal(a)
			}
			if len(e.requests) != 2 || e.requests[0].ReconcileOnly || !e.requests[1].ReconcileOnly {
				t.Fatal("retry replayed add")
			}
		})
	}
}

func TestOptionalJournalConcurrentFinalizationReusesDownload(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	var reconciledID int64
	var other *Manager
	e := &attemptEngine{add: func(r AddRequest) (string, error) {
		if r.ReconcileOnly {
			return r.AttemptMarker, nil
		}
		// The first request was accepted, but its response has not returned.
		var err error
		reconciledID, err = other.Grab(ctx, w.slotID, w.relID, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		return r.AttemptMarker, nil
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	other = &Manager{Store: w.s, Engine: m.Engine, Now: w.now, Live: w}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil || id != reconciledID || id == 0 {
		t.Fatal("concurrent finalization was not idempotent", id, reconciledID, err)
	}
	var count int
	if err := w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	got, _ := m.Attempts(ctx, d.ID)
	if len(got) != 1 || got[0].State != "resolved" || len(e.requests) != 2 || !e.requests[1].ReconcileOnly {
		t.Fatal("concurrent recovery replayed or lost intent")
	}
	if _, count, err := w.s.Events(ctx, database.EventFilter{Kinds: []string{"download_sent"}}); err != nil || count != 1 {
		t.Fatal("concurrent finalization duplicated send history", count, err)
	}
	for _, topic := range []string{"event", "slot", "download"} {
		count := 0
		for _, got := range w.topics {
			if got == topic {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("concurrent finalization published %s %d times", topic, count)
		}
	}
}

func TestOptionalJournalPackDoesNotBindRecreatedSecondarySlot(t *testing.T) {
	w, m, _ := journalWorld(t)
	ctx := context.Background()
	var second mediafiles.Slot
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var first mediafiles.Slot
		if err := database.Get(ctx, tx, "slots", w.slotID, &first); err != nil {
			return err
		}
		var item mediafiles.Item
		if err := database.Get(ctx, tx, "items", first.ItemID, &item); err != nil {
			return err
		}
		item = mediafiles.Item{TitleID: item.TitleID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", &item, domain.System); err != nil {
			return err
		}
		second = mediafiles.Slot{ItemID: item.ID, VersionID: first.VersionID, State: mediafiles.SlotChosen, ChosenReleaseID: &w.relID}
		if _, err := database.Insert(tx, "slots", &second, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE releases SET matched_slot_ids = ?, verdict = json_set(verdict, '$.pick_slot_ids', json(?)) WHERE id = ?",
			jsonIDs([]int64{w.slotID, second.ID}), jsonIDs([]int64{w.slotID, second.ID}), w.relID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	originalID, originalCreated := second.ID, second.CreatedAt
	e := &attemptEngine{add: func(r AddRequest) (string, error) {
		if r.ReconcileOnly {
			t.Fatal("fresh pack send unexpectedly reconciled")
		}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("DELETE FROM slots WHERE id = ?", second.ID); err != nil {
				return err
			}
			if _, err := database.Insert(tx, "slots", &second, domain.System); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE slots SET created_at = ? WHERE id = ?", database.FormatTime(originalCreated.Add(time.Hour)), second.ID)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if second.ID != originalID {
			t.Fatal("fixture did not reuse the secondary Slot id")
		}
		return r.AttemptMarker, nil
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	dl := w.download(t, id)
	if len(dl.SlotIDs) != 1 || dl.SlotIDs[0] != w.slotID {
		t.Fatal("late response attached a recreated secondary Slot", dl.SlotIDs)
	}
	if err := database.Get(ctx, w.s.Read(), "slots", second.ID, &second); err != nil {
		t.Fatal(err)
	}
	if second.State != mediafiles.SlotChosen || second.ActiveDownloadID != nil || second.ChosenReleaseID == nil || *second.ChosenReleaseID != w.relID {
		t.Fatal("late response moved the recreated secondary Slot", second)
	}
}

func TestOptionalJournalPackSharesOneAppend(t *testing.T) {
	w, m, _ := journalWorld(t)
	ctx := context.Background()
	var otherSlot int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var versionID, titleID int64
		if err := tx.QueryRow("SELECT s.version_id, i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", w.slotID).Scan(&versionID, &titleID); err != nil {
			return err
		}
		item := &mediafiles.Item{TitleID: titleID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", item, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: item.ID, VersionID: versionID, State: mediafiles.SlotChosen, ChosenReleaseID: &w.relID}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		otherSlot = sl.ID
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", w.relID, w.slotID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE releases SET matched_slot_ids = ?, verdict = json_set(verdict, '$.pick_slot_ids', json(?)) WHERE id = ?", jsonIDs([]int64{w.slotID, otherSlot}), jsonIDs([]int64{w.slotID, otherSlot}), w.relID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	e := &attemptEngine{add: func(r AddRequest) (string, error) {
		if !r.ReconcileOnly {
			close(entered)
			<-release
		}
		return r.AttemptMarker, nil
	}}
	m.Engine = func(domain.Downloader) (Engine, error) { return e, nil }
	other := &Manager{Store: w.s, Engine: m.Engine, Now: w.now}
	type result struct {
		id  int64
		err error
	}
	done := make(chan result, 1)
	go func() { id, err := m.Grab(ctx, w.slotID, w.relID, domain.System); done <- result{id, err} }()
	<-entered
	secondID, err := other.Grab(ctx, otherSlot, w.relID, domain.System)
	close(release)
	first := <-done
	if err != nil || first.err != nil || first.id != secondID || secondID == 0 {
		t.Fatal(first, secondID, err)
	}
	appends := 0
	for _, r := range e.requests {
		if !r.ReconcileOnly {
			appends++
		}
		if r.AttemptMarker != e.requests[0].AttemptMarker {
			t.Fatal("pack claimed separate identities")
		}
	}
	if appends != 1 {
		t.Fatal("pack appended", appends)
	}
	var count int
	if err := w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if len(w.download(t, secondID).SlotIDs) != 2 {
		t.Fatal("pack lost a Slot")
	}
}

func TestOptionalJournalDifferentVersionsKeepSeparateIntents(t *testing.T) {
	w, m, d := journalWorld(t)
	ctx := context.Background()
	var secondSlot int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var sl mediafiles.Slot
		if err := database.Get(ctx, tx, "slots", w.slotID, &sl); err != nil {
			return err
		}
		var version mediafiles.Version
		if err := database.Get(ctx, tx, "versions", sl.VersionID, &version); err != nil {
			return err
		}
		version.ID, version.Label, version.FolderName, version.IsMain = 0, "Other", "Other", false
		if _, err := database.Insert(tx, "versions", &version, domain.System); err != nil {
			return err
		}
		sl.ID, sl.VersionID = 0, version.ID
		if _, err := database.Insert(tx, "slots", &sl, domain.System); err != nil {
			return err
		}
		secondSlot = sl.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first, err := m.claimAttempt(ctx, d, w.relID, w.slotID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.claimAttempt(ctx, d, w.relID, secondSlot)
	if err != nil || !second.created || second.marker == first.marker {
		t.Fatal("Versions shared an intent", err)
	}
}

func journalWorld(t *testing.T) (*world, *Manager, domain.Downloader) {
	t.Helper()
	w, m := setup(t)
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE releases SET protocol = 'usenet', info_hash = NULL"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE downloaders SET adapter = 'nzbget', protocol = 'usenet'")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, m, w.downloader(t, 1)
}
