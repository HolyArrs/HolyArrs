package downloader

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/outside"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// KindGrab is the Job kind that sends a chosen Release to a Downloader.
const KindGrab = "grab"

// Publisher sends live events (live.Hub fits).
type Publisher interface {
	Publish(topic string, data any) uint64
}

// Manager sends Releases to Downloaders and keeps Download records in step.
type Manager struct {
	Store    *store.Store
	Live     Publisher                                 // may be nil
	Engine   func(d domain.Downloader) (Engine, error) // nil means the built-in adapters
	SavePath string                                    // where the Downloader saves, as it sees it
	// StallAfter and ActAfter override the settings stalled_after_hours
	// and stalled_act_hours (tests); zero reads the settings.
	StallAfter  time.Duration
	ActAfter    time.Duration
	VanishGrace time.Duration                                    // a sent Download may be unlisted this long; default 2m
	Active      time.Duration                                    // poll interval with active Downloads; default 2s
	Idle        time.Duration                                    // poll interval when idle; default 30s
	HealthEvery time.Duration                                    // status check of every Downloader; default 1m
	Now         func() time.Time                                 // for tests
	Sleep       func(ctx context.Context, d time.Duration) error // for tests; nil waits for real
	OnFinished  func(ctx context.Context, downloadID int64)      // start the Import
	// OnFailed is told about each Slot whose Download failed and that should
	// be searched again (the search then picks the next Release, or the
	// Slot needs a decision).
	OnFailed func(ctx context.Context, slotID int64)
	// Link gives the link to send for a Release: its stored link with the
	// Source's key put back from the vault. Nil sends the stored link.
	Link func(ctx context.Context, rel domain.Release) (string, error)
	// FromVault reads a Downloader's key or password from the vault.
	FromVault func(ctx context.Context, id int64) (string, error)

	mu               sync.Mutex
	engines          map[int64]Engine
	engineGeneration map[int64]uint64
	wake             chan struct{}
	rr               map[string]int           // round robin turn per protocol and priority
	hosts            map[string]chan struct{} // one send at a time per host (a slot of one)
	lastSend         map[string]time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) publish(topic string, data any) {
	if m.Live != nil {
		m.Live.Publish(topic, data)
	}
}

// Forget drops the cached engine of a Downloader, after an edit.
func (m *Manager) Forget(id int64) {
	m.mu.Lock()
	delete(m.engines, id)
	if m.engineGeneration == nil {
		m.engineGeneration = map[int64]uint64{}
	}
	m.engineGeneration[id]++
	m.mu.Unlock()
}

func (m *Manager) engine(ctx context.Context, d domain.Downloader) (Engine, error) {
	m.mu.Lock()
	if e := m.engines[d.ID]; e != nil {
		m.mu.Unlock()
		return e, nil
	}
	generation := m.engineGeneration[d.ID]
	m.mu.Unlock()
	var e Engine
	var err error
	if m.Engine != nil {
		e, err = m.Engine(d)
	} else {
		e, err = m.EngineFor(ctx, d)
		if err == nil {
			instance := fmt.Sprintf("%p", m)
			if m.Store != nil {
				instance = m.Store.Dir()
			}
			e = protectEngine(e, d, instance)
		}
	}
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if generation != m.engineGeneration[d.ID] {
		m.mu.Unlock()
		return nil, errors.New("the Downloader changed while connecting; try again")
	}
	if m.engines == nil {
		m.engines = map[int64]Engine{}
	}
	if existing := m.engines[d.ID]; existing != nil {
		m.mu.Unlock()
		return existing, nil
	}
	m.engines[d.ID] = e
	m.mu.Unlock()
	return e, nil
}

// EngineFor builds the built-in adapter for a Downloader, with its secret
// from the vault and a network policy that keeps it off the local network
// unless the Downloader may reach it.
func (m *Manager) EngineFor(ctx context.Context, d domain.Downloader) (Engine, error) {
	secret := ""
	if d.SecretRef != nil {
		if m.FromVault == nil {
			return nil, errors.New("the Downloader's key cannot be read: the vault is not open")
		}
		var err error
		if secret, err = m.FromVault(ctx, *d.SecretRef); err != nil {
			return nil, errors.New("the Downloader's key cannot be read from the vault")
		}
	}
	h := Client(d.URL, d.AllowLocal)
	switch d.Adapter {
	case "qbittorrent":
		return NewQbittorrentKey(d.URL, secret, h), nil
	case "sabnzbd":
		return NewSabnzbd(d.URL, secret, h), nil
	case "transmission":
		e := NewTransmission(d.URL, d.Username, secret, h)
		e.category = d.Category
		return e, nil
	case "nzbget":
		e := NewNZBGet(d.URL, d.Username, secret, h)
		e.Category = d.Category
		return e, nil
	case "deluge":
		e := NewDeluge(d.URL, secret, h)
		e.Category = d.Category
		return e, nil
	}
	return nil, fmt.Errorf("no engine for adapter %q", d.Adapter)
}

// HandleGrab runs a grab Job: params release_id, subject the Slot.
func (m *Manager) HandleGrab(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "slot" {
		return errors.New("grab needs a slot subject")
	}
	relID, _ := r.Job.Params["release_id"].(float64)
	id, err := m.Grab(ctx, r.Job.Subject.ID, int64(relID), domain.Actor{Kind: "job", ID: r.Job.ID})
	if err != nil {
		return err
	}
	r.SetResult(map[string]any{"download_id": id})
	return nil
}

// Grab sends a Release to a Downloader for its protocol (by priority, equal
// ones in turn, resting ones skipped), records the Download and moves the
// Slot to downloading.
func (m *Manager) Grab(ctx context.Context, slotID, releaseID int64, by domain.Actor) (int64, error) {
	db := m.Store.Read()
	var rel domain.Release
	if err := store.Get(ctx, db, "releases", releaseID, &rel); err != nil {
		return 0, fmt.Errorf("release %d: %w", releaseID, err)
	}
	var sl domain.Slot
	if err := store.Get(ctx, db, "slots", slotID, &sl); err != nil {
		return 0, fmt.Errorf("slot %d: %w", slotID, err)
	}
	// A stopped semantic pair remains terminal even after abandonment has
	// returned its Slot to a state where a person can choose another Release.
	attempt, err := unfinishedAttempt(ctx, db, releaseID, slotID)
	if err != nil {
		return 0, err
	}
	if attempt != nil && attempt.State == "abandoned" {
		return 0, ErrAttemptStopped
	}
	if sl.State != domain.SlotChosen {
		if sl.ActiveDownloadID != nil && sl.State == domain.SlotDownloading {
			var prior domain.Download
			if err := store.Get(ctx, db, "downloads", *sl.ActiveDownloadID, &prior); err == nil && prior.ReleaseID != nil && *prior.ReleaseID == releaseID && slices.Contains(prior.SlotIDs, slotID) && isActive(prior.State) {
				return prior.ID, nil
			}
		}
		return 0, fmt.Errorf("slot %d is %s, not chosen", slotID, sl.State)
	}
	if sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != releaseID {
		return 0, errors.New("the Slot is no longer chosen for this Release")
	}
	all, err := store.Select[domain.Downloader](ctx, db, "downloaders", "protocol = ? AND removed = 0 ORDER BY priority, id", rel.Protocol)
	if err != nil {
		return 0, err
	}
	hash := ""
	if rel.InfoHash != nil && source.ValidInfoHash(strings.ToLower(*rel.InfoHash)) {
		hash = strings.ToLower(*rel.InfoHash)
	}
	now := m.now()
	// Unknown outcomes pin the original Downloader before ordering or backoff.
	// An empty lookup after a restart never permits a failover append.
	// A retry after a kill finds the Download the first try wrote, or the
	// torrent already in the Downloader: it must not add a second one.
	var dl *domain.Download
	if prior, err := store.Select[domain.Download](ctx, db, "downloads",
		"release_id = ? AND state IN ("+activeStates+", 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?)"+
			" AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = downloads.id AND i.state = 'rejected') ORDER BY id DESC LIMIT 1",
		rel.ID, slotID); err != nil {
		return 0, err
	} else if len(prior) == 1 {
		dl = &prior[0]
	}
	var d domain.Downloader
	if dl != nil {
		if err := store.Get(ctx, db, "downloaders", dl.DownloaderID, &d); err != nil {
			return 0, err
		}
	} else {
		var dls []domain.Downloader
		var skipped []string
		if attempt != nil {
			var original domain.Downloader
			if err := store.Get(ctx, db, "downloaders", attempt.DownloaderID, &original); err != nil {
				return 0, errors.Join(ErrUncertain, err)
			}
			original.Category = attempt.category
			dls = []domain.Downloader{original}
		} else {
			dls, skipped = m.order(all, now)
		}
		if len(dls) == 0 {
			if len(skipped) > 0 {
				return 0, fmt.Errorf("no %s Downloader can take it now: %s", rel.Protocol, strings.Join(skipped, "; "))
			}
			return 0, fmt.Errorf("no Downloader for %s", rel.Protocol)
		}
		// When one cannot take the Release the next of the same protocol is
		// tried (setting try_next_downloader, default on), and the error
		// says why each one failed.
		tryNext := true
		var v string
		if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'what_to_pick' AND key = 'try_next_downloader'").Scan(&v) == nil && v == "false" {
			tryNext = false
		}
		var fails []string
		for i := 0; dl == nil && i < len(dls); i++ {
			if i > 0 && !tryNext {
				break
			}
			d = dls[i]
			ext, err := m.send(ctx, d, rel, slotID, hash)
			if err != nil {
				if errors.Is(err, ErrUncertain) || errors.Is(err, ErrAttemptStopped) {
					return 0, err
				}
				fails = append(fails, err.Error())
				continue
			}
			dl = &domain.Download{DownloaderID: d.ID, ReleaseID: &rel.ID, SlotIDs: []int64{slotID}, ExternalID: ext, State: "sent",
				FilesSeen: []domain.SeenFile{}, SavePath: m.SavePath, StartedBy: by, ProgressAt: &now}
		}
		if dl == nil {
			return 0, errors.New(strings.Join(fails, "; "))
		}
	}
	// The Download row and the Slot move are one write, so a kill cannot
	// leave a Download with the Slot still chosen.
	moved := []int64{slotID}
	adopted := false
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		// Another caller can reconcile an accepted send before its original
		// caller returns. Adopt that committed pair instead of resetting it.
		if dl.ID == 0 {
			prior, err := store.Select[domain.Download](ctx, tx, "downloads", `downloader_id = ? AND external_id = ? AND release_id = ?
				AND state IN (`+activeStates+`, 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?)
				AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = downloads.id AND i.state = 'rejected') LIMIT 1`, d.ID, dl.ExternalID, rel.ID, slotID)
			if err != nil {
				return err
			}
			if len(prior) == 1 {
				var active sql.NullInt64
				var state string
				if err := tx.QueryRowContext(ctx, "SELECT active_download_id, state FROM slots WHERE id = ?", slotID).Scan(&active, &state); err != nil {
					return err
				}
				if active.Valid && active.Int64 == prior[0].ID && state == string(domain.SlotDownloading) {
					dl = &prior[0]
					adopted = true
					return nil
				}
			}
		}
		// Resolve the intent in this same transaction as the Download and Slot.
		// If an owner abandoned it while the request was in flight, roll back.
		var intentID int64
		if journalAdapter(d.Adapter) {
			a, err := unfinishedAttempt(ctx, tx, rel.ID, slotID)
			if err != nil {
				return err
			}
			if a == nil && dl.ID == 0 {
				// A changed Release or Version snapshot cannot adopt the
				// response of the original send by reusing its numeric id.
				return ErrUncertain
			}
			if a != nil {
				intentID = a.ID
				if a.State != "pending" {
					return ErrAttemptStopped
				}
				if a.DownloaderID != d.ID || d.Adapter == "nzbget" && a.marker != dl.ExternalID || d.Adapter != "nzbget" && a.expectedID != "" && !strings.EqualFold(a.expectedID, dl.ExternalID) {
					return ErrUncertain
				}
				if _, err := tx.ExecContext(ctx, "UPDATE downloader_attempts SET state = 'resolved', updated_at = ? WHERE id = ? AND state = 'pending'", m.now().Format(time.RFC3339Nano), a.ID); err != nil {
					return err
				}
			}
		}
		var current domain.Slot
		if err := store.Get(ctx, tx, "slots", slotID, &current); err != nil {
			return err
		}
		if current.State != domain.SlotChosen || current.ChosenReleaseID == nil || *current.ChosenReleaseID != rel.ID ||
			!current.CreatedAt.Equal(sl.CreatedAt) || current.VersionID != sl.VersionID {
			return errors.New("the Slot changed while sending this Release")
		}
		// A season pack or multi-episode Release chosen for several Slots is
		// one Download for all of them (the other Slots still chosen for
		// this same Release: an old pick list can name a Slot chosen since
		// for another one).
		// Only Slots of the grabbed Slot's Version: one Download never fills
		// two Versions (the feed check may choose one Release for both).
		if many := source.PickSlots(rel); len(many) > 1 && slices.Contains(many, slotID) {
			var version int64
			if err := tx.QueryRow("SELECT version_id FROM slots WHERE id = ?", slotID).Scan(&version); err != nil {
				return err
			}
			for _, id := range many {
				var st string
				var chosenFor, versionOf sql.NullInt64
				// A late response may only attach the identities covered by
				// its intent, even when SQLite has reused a Slot's numeric id.
				if id == slotID || tx.QueryRow(`SELECT state, chosen_release_id, version_id FROM slots s WHERE id = ?
					AND (? = 0 OR EXISTS (SELECT 1 FROM downloader_attempts a, json_each(a.slot_snapshots) j
					WHERE a.id = ? AND json_extract(j.value, '$.id') = s.id AND json_extract(j.value, '$.created_at') = s.created_at))`, id, intentID, intentID).Scan(&st, &chosenFor, &versionOf) != nil ||
					st != string(domain.SlotChosen) || chosenFor.Int64 != rel.ID || versionOf.Int64 != version {
					continue
				}
				moved = append(moved, id)
				if !slices.Contains(dl.SlotIDs, id) {
					dl.SlotIDs = append(dl.SlotIDs, id)
				}
			}
			if dl.ID > 0 && len(moved) > 1 {
				if _, err := tx.Exec("UPDATE downloads SET slot_ids = ? WHERE id = ?", jsonIDs(dl.SlotIDs), dl.ID); err != nil {
					return err
				}
			}
		}
		if dl.ID == 0 {
			// The Downloader may give back the id of an old Download that was
			// not reused (its Import was rejected): that row starts afresh.
			// A row another Slot is still following is never taken over.
			var old int64
			var oldState string
			if tx.QueryRow("SELECT id, state FROM downloads WHERE downloader_id = ? AND external_id = ?", dl.DownloaderID, dl.ExternalID).Scan(&old, &oldState) == nil {
				if isActive(oldState) {
					return fmt.Errorf("%s gave the id of Download %d, which is still active", d.Name, old)
				}
				dl.ID = old
				if err := store.Update(tx, "downloads", dl, by); err != nil {
					return err
				}
			} else if _, err := store.Insert(tx, "downloads", dl, by); err != nil {
				return err
			}
		}
		for _, id := range moved {
			if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, id); err != nil {
				return err
			}
			if err := store.MoveSlotTx(tx, id, domain.SlotDownloading, "sent "+rel.Name+" to "+d.Name, by); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if adopted {
		return dl.ID, nil // The caller that committed it owns its notifications.
	}
	m.event(ctx, dl, "download_sent", fmt.Sprintf("Sent %s to %s", rel.Name, d.Name))
	for _, id := range moved {
		m.publish("slot", map[string]any{"id": id, "state": domain.SlotDownloading})
	}
	m.publish("download", dl)
	m.Poke()
	return dl.ID, nil
}

// send hands a Release to one Downloader and returns its id there.
func (m *Manager) send(ctx context.Context, d domain.Downloader, rel domain.Release, slotID int64, hash string) (ext string, err error) {
	a, err := m.claimAttempt(ctx, d, rel.ID, slotID)
	if err != nil {
		return "", err
	}
	if a != nil {
		if a.State == "resolved" {
			return a.marker, nil
		}
		d.Category = a.category
		defer func() {
			if err == nil {
				return // Grab resolves it atomically with the Download row.
			}
			if !a.created || errors.Is(err, ErrUncertain) {
				err = errors.Join(ErrUncertain, err)
				return
			}
			// A definite pre-append refusal may be retried. Failed cleanup is
			// conservative: retain the intent and suppress failover.
			cleanupErr := m.Store.Write(ctx, func(tx *sql.Tx) error {
				res, e := tx.ExecContext(ctx, "UPDATE downloader_attempts SET state = 'resolved', updated_at = ? WHERE id = ? AND state = 'pending'", m.now().Format(time.RFC3339Nano), a.ID)
				if e != nil {
					return e
				}
				n, e := res.RowsAffected()
				if e == nil && n != 1 {
					return ErrAttemptStopped
				}
				return e
			})
			if cleanupErr != nil {
				err = errors.Join(ErrUncertain, err, cleanupErr)
			}
		}()
	}
	e, err := m.engine(ctx, d)
	if err != nil {
		return "", fmt.Errorf("%s: %w", d.Name, err)
	}
	// A job found again by name is taken as this Release only when no other
	// Download row holds its id: a failed or vanished one must not come back,
	// and a row for another Release or Slot is not this one.
	skip := map[string]bool{}
	rows, err := m.Store.Read().QueryContext(ctx, `SELECT external_id FROM downloads WHERE downloader_id = ? AND NOT (release_id IS ?
		AND state IN (`+activeStates+`, 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?))`, d.ID, rel.ID, slotID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			skip[id] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	if a != nil && !a.created {
		// A recovery is only a lookup. It needs neither the original Source
		// nor a send-spacing reservation, and may never fetch or append.
		ext, err = e.Add(ctx, AddRequest{Name: rel.Name, Category: a.category, Skip: skip, AttemptMarker: a.marker, InfoHash: a.expectedID, ReconcileOnly: true})
		if err != nil {
			if unreachable(err) {
				m.downloaderFailed(ctx, d, err)
			}
			return "", err
		}
		if ext == "" {
			return "", ErrUncertain
		}
		m.downloaderOK(ctx, d)
		return ext, nil
	}
	if hash != "" {
		if j, err := e.Get(ctx, hash); err == nil {
			if skip[j.ID] || (d.Adapter == "transmission" || d.Adapter == "deluge") && j.Category != d.Category {
				return "", errors.New("the torrent belongs to another download or category")
			}
			if j.ID != "" {
				return j.ID, nil
			}
			return hash, nil
		}
	}
	link := rel.LinkRef
	if m.Link != nil {
		if link, err = m.Link(ctx, rel); err != nil {
			return "", err
		}
	}
	allowLocal := false
	if d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "nzbget" {
		var src domain.Source
		if err := store.Get(ctx, m.Store.Read(), "sources", rel.SourceID, &src); err != nil {
			return "", errors.New("the Release's Source could not be read")
		}
		allowLocal = src.AllowLocal
		if !strings.HasPrefix(link, "magnet:") && src.ProxyURL != "" {
			return "", errors.New("this Downloader cannot yet fetch Release files through a Source proxy")
		}
	}
	done, err := m.space(ctx, d.URL)
	if err != nil {
		return "", err
	}
	req := AddRequest{URL: link, Name: rel.Name, Category: d.Category, SavePath: m.SavePath, InfoHash: hash, Skip: skip,
		AllowLocal: allowLocal, OperationKey: fmt.Sprintf("%d:%d:%d", d.ID, rel.ID, slotID)}
	if a != nil {
		req.AttemptMarker, req.ReconcileOnly = a.marker, !a.created
	}
	ext, err = e.Add(ctx, req)
	done()
	if err != nil {
		// Only a Downloader that did not answer, or turned down the
		// sign-in, rests; one that answered and refused this Release
		// (a bad link, no category) stays ready for the next one.
		if unreachable(err) {
			m.downloaderFailed(ctx, d, err)
		}
		return "", fmt.Errorf("sending to %s: %w", d.Name, err)
	}
	if ext == "" {
		if a != nil {
			return "", ErrUncertain
		}
		return "", fmt.Errorf("%s gave no id for the Release (no info hash)", d.Name)
	}
	m.downloaderOK(ctx, d)
	return ext, nil
}

func (m *Manager) event(ctx context.Context, d *domain.Download, kind, summary string) {
	id := d.ID
	data := map[string]any{"state": d.State, "progress": d.Progress, "save_path": d.SavePath}
	if d.StateReason != nil {
		data["reason"] = *d.StateReason
	}
	db := m.Store.Read()
	var name string
	if d.ReleaseID != nil && db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *d.ReleaseID).Scan(&name) == nil {
		data["release"], data["release_id"] = name, *d.ReleaseID
	}
	if db.QueryRowContext(ctx, "SELECT name FROM downloaders WHERE id = ?", d.DownloaderID).Scan(&name) == nil {
		data["downloader"] = name
	}
	e := &domain.Event{Kind: kind, DownloadID: &id, Summary: summary, Data: data}
	if len(d.SlotIDs) > 1 {
		data["slot_ids"] = d.SlotIDs // a pack: Alerts name every episode
	}
	if len(d.SlotIDs) > 0 {
		s := d.SlotIDs[0]
		e.SlotID = &s
		e.GroupKey = fmt.Sprintf("slot:%d", s)
		var titleID int64
		if db.QueryRowContext(ctx, "SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", s).Scan(&titleID) == nil {
			e.TitleID = &titleID
		}
	}
	if m.Store.AddEvent(ctx, e) == nil {
		m.publish("event", e)
	}
}

// activeStates are the Download states the sync follows.
const activeStates = "'sent', 'queued', 'downloading', 'stalled', 'paused'"

// SyncOnce reads every active Download from its Downloader and records what
// changed, after asking again for any removal a Downloader missed (see
// retryRemovals). It returns how many Downloads are still active. With nothing
// active and no removal pending it reads nothing and writes nothing.
func (m *Manager) SyncOnce(ctx context.Context) (int, error) {
	m.retryRemovals(ctx)
	db := m.Store.Read()
	dls, err := store.Select[domain.Download](ctx, db, "downloads", "state IN ("+activeStates+") ORDER BY id")
	if err != nil || len(dls) == 0 {
		return 0, err
	}
	byDownloader := map[int64][]domain.Download{}
	for _, d := range dls {
		byDownloader[d.DownloaderID] = append(byDownloader[d.DownloaderID], d)
	}
	st := m.settings(ctx)
	active := 0
	var errs []error
	for did, list := range byDownloader {
		var dr domain.Downloader
		if err := store.Get(ctx, db, "downloaders", did, &dr); err != nil {
			errs = append(errs, err)
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		jobsIn, err := e.List(ctx, dr.Category)
		if err != nil {
			m.downloaderFailed(ctx, dr, err)
			errs = append(errs, err)
			active += len(list)
			continue
		}
		m.downloaderOK(ctx, dr)
		have := map[string]Job{}
		for _, j := range jobsIn {
			have[j.ID] = j
		}
		for _, d := range list {
			j, ok := have[d.ExternalID]
			if ok && j.State == StateFinished {
				if full, err := e.Get(ctx, d.ExternalID); err == nil {
					j = full
				}
				if len(j.Files) == 0 {
					j.SavePath, j.Files = readFinished(j, dr.PathMap, mapLocal(m.SavePath, dr.PathMap))
				}
			} else if ok && st.failDangerous && dr.Protocol == "torrent" && len(d.FilesSeen) == 0 && j.State != StateFailed {
				// The file list once, as soon as the torrent has it, so a
				// fake fails before it downloads (4.3.5).
				if full, err := e.Get(ctx, d.ExternalID); err == nil && len(full.Files) > 0 {
					j.Files = full.Files
				}
			}
			if m.apply(ctx, dr, d, j, ok, st) {
				active++
			}
		}
	}
	return active, errors.Join(errs...)
}

// apply records one Download's new state; it reports whether it is still active.
func (m *Manager) apply(ctx context.Context, dr domain.Downloader, d domain.Download, j Job, found bool, st settings) bool {
	now := m.now()
	next := d
	last := d.UpdatedAt
	if d.ProgressAt != nil {
		last = *d.ProgressAt
	}
	failReason := ""
	switch {
	case !found && d.State == "sent" && now.Sub(last) < m.vanishGrace():
		// Added by URL: the Downloader is still fetching the file.
		return true
	case !found:
		next.State = "vanished"
		next.StateReason = strPtr("removed outside HolyArrs: the Downloader no longer has it")
	case st.failDangerous && j.State != StateFailed && len(fakeFiles(j.Files, dr.Protocol)) > 0:
		// Known as soon as the Downloader lists the files (4.3.5), so a fake
		// is not downloaded to the end.
		next.State = "failed"
		failReason = fakeReason(j.Files, dr.Protocol)
		next.StateReason = &failReason
	case j.State == StateFinished:
		next.State, next.Progress = "finished", 1
		next.FinishedAt = &now
		next.Speed, next.UpSpeed, next.ETA = 0, 0, nil
	case j.State == StateFailed:
		next.State = "failed"
		failReason = "the Downloader says " + j.Detail
		next.StateReason = &failReason
	default:
		next.Progress, next.Speed = min(max(j.Progress, 0), 1), j.Speed
		next.UpSpeed, next.Seeds, next.Peers = j.UpSpeed, j.Seeds, j.Peers
		if j.ETA > 0 {
			eta := j.ETA
			next.ETA = &eta
		} else {
			next.ETA = nil
		}
		// A higher mark restarts the stall timer, and so does moving again
		// after waiting or being paused, which never count.
		waited := d.State == "queued" || d.State == "paused"
		if j.Progress > d.Progress || d.ProgressAt == nil || waited && j.State == StateDownloading {
			last = now
			next.ProgressAt = &now
		}
		quiet := now.Sub(last)
		switch {
		case j.State == StateQueued:
			next.State = "queued"
		case j.State == StatePaused:
			next.State = "paused"
		case next.Progress >= 1:
			// Repairing or unpacking at full progress is never stalled.
			next.State = "downloading"
			next.StateReason = nil
		case quiet >= st.actAfter:
			next.State = "failed"
			failReason = fmt.Sprintf("no progress for %s", hours(quiet))
			next.StateReason = &failReason
		case quiet >= st.stallAfter:
			next.State = "stalled"
			next.StateReason = strPtr(fmt.Sprintf("no progress for %s", hours(quiet)))
		default:
			next.State = "downloading"
			next.StateReason = nil
		}
		if next.Progress < d.Progress {
			// Recorded, but only a higher mark resets the stall timer.
			next.Progress = d.Progress
		}
		if next.StartedAt == nil && next.State == "downloading" {
			next.StartedAt = &now
		}
	}
	if found {
		if j.SavePath != "" {
			next.SavePath = j.SavePath
		}
		if len(j.Files) > 0 {
			next.FilesSeen = make([]domain.SeenFile, len(j.Files))
			for i, f := range j.Files {
				next.FilesSeen[i] = domain.SeenFile{Name: f.Name, Size: f.Size}
			}
		}
	}
	if next.State == "failed" || next.State == "vanished" {
		m.failed(ctx, dr, d, next, st, domain.System)
		return false
	}
	if st.failDangerous && len(d.FilesSeen) == 0 && len(j.Files) > 0 {
		// A web shortcut beside the media is common on public torrents:
		// noted once, when the file list is first seen (4.3.5).
		if sc := shortcuts(j.Files); len(sc) > 0 {
			m.event(ctx, &next, "download_shortcut", "Web shortcut in the download, left alone: "+strings.Join(sc, ", "))
		}
	}
	changed := next.State != d.State || next.Progress != d.Progress || next.Speed != d.Speed || next.UpSpeed != d.UpSpeed || next.Seeds != d.Seeds || next.Peers != d.Peers || !eqTime(next.ProgressAt, d.ProgressAt) ||
		!eqPtr(next.ETA, d.ETA) || next.SavePath != d.SavePath || len(next.FilesSeen) != len(d.FilesSeen)
	if !changed {
		return isActive(next.State)
	}
	// The write holds only while the row is still in the state this
	// snapshot read: Mark failed (or another path) may have moved it since,
	// and a stale snapshot is dropped.
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		return store.UpdateWhere(tx, "downloads", &next, domain.System, "state = ?", d.State)
	}); err != nil {
		return !errors.Is(err, store.ErrNotFound)
	}
	m.publish("download", next)
	if next.State != d.State {
		m.stateMoved(ctx, d.State, &next)
	}
	return isActive(next.State)
}

func hours(d time.Duration) string {
	if d >= time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	return fmt.Sprintf("%d minutes", int(d.Minutes()))
}

func (m *Manager) stateMoved(ctx context.Context, from string, d *domain.Download) {
	m.event(ctx, d, "download_"+d.State, fmt.Sprintf("Download %s: %s", d.ExternalID[:min(8, len(d.ExternalID))], d.State))
	var to domain.SlotState
	reason := ""
	if d.StateReason != nil {
		reason = *d.StateReason
	}
	switch d.State {
	case "stalled":
		to = domain.SlotStalled
	case "downloading", "finished":
		// Back from stalled, or finished straight from stalled: the Slots
		// wait for this download again, so the import finds them.
		if from == "stalled" {
			to = domain.SlotDownloading
		}
	}
	for _, sid := range d.SlotIDs {
		if to != "" {
			if err := m.Store.MoveSlot(ctx, sid, to, reason, domain.System); err == nil {
				m.publish("slot", map[string]any{"id": sid, "state": to, "reason": reason})
			}
		}
	}
	if d.State == "finished" && m.OnFinished != nil {
		m.OnFinished(ctx, d.ID)
	}
}

func isActive(state string) bool {
	switch state {
	case "sent", "queued", "downloading", "stalled", "paused":
		return true
	}
	return false
}

// vanishGrace is how long a sent Download may be missing from its
// Downloader before it counts as vanished.
func (m *Manager) vanishGrace() time.Duration {
	if m.VanishGrace > 0 {
		return m.VanishGrace
	}
	return 2 * time.Minute
}

// Interval is how long to wait before the next sync.
func (m *Manager) Interval(active int) time.Duration {
	if active > 0 {
		if m.Active > 0 {
			return m.Active
		}
		return 2 * time.Second
	}
	if m.Idle > 0 {
		return m.Idle
	}
	return 30 * time.Second
}

// Poke makes the sync loop run now.
func (m *Manager) Poke() {
	m.mu.Lock()
	if m.wake == nil {
		m.wake = make(chan struct{}, 1)
	}
	w := m.wake
	m.mu.Unlock()
	select {
	case w <- struct{}{}:
	default:
	}
}

// Run syncs until ctx ends: every 2s while Downloads are active, 30s when
// idle, and checks every Downloader once a minute.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	if m.wake == nil {
		m.wake = make(chan struct{}, 1)
	}
	w := m.wake
	m.mu.Unlock()
	every := m.HealthEvery
	if every <= 0 {
		every = time.Minute
	}
	var checked time.Time
	for {
		if time.Since(checked) >= every {
			m.CheckAll(ctx)
			checked = time.Now()
		}
		n, _ := m.SyncOnce(ctx)
		t := time.NewTimer(min(m.Interval(n), every))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-w:
			t.Stop()
		case <-t.C:
		}
	}
}

func strPtr(s string) *string { return &s }

// unreachable says whether err means the Downloader did not answer (a
// network error or timeout, an address the policy refused) or turned down
// the sign-in.
func unreachable(err error) bool {
	var ne net.Error
	var status adapterHTTPError
	return errors.Is(err, ErrAuth) || errors.Is(err, outside.ErrSuppressed) || errors.Is(err, errDownloaderUnavailable) ||
		errors.As(err, &status) && (status.status == 408 || status.status == 429 || status.status >= 500) ||
		errors.Is(err, source.ErrAddressRefused) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne)
}

func eqPtr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func jsonIDs(ids []int64) string {
	b, _ := json.Marshal(ids)
	return string(b)
}
