package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// HandleGrab runs a grab Job: params release_id, subject the Slot.
func (m *Manager) HandleGrab(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "slot" {
		return errors.New("grab needs a slot subject")
	}
	relID, _ := r.Job.Params["release_id"].(float64)
	by := domain.Actor{Kind: "job", ID: r.Job.ID}
	if r.Job.AskedBy.Kind == "user" {
		// The person who picked it is who the download is for.
		by = r.Job.AskedBy
	}
	id, err := m.Grab(ctx, r.Job.Subject.ID, int64(relID), by)
	var te *TagsError
	if errors.As(err, &te) || errors.Is(err, ErrOutsideTorrent) {
		// No Downloader takes the title's tags, or the torrent sits in the Downloader
		// outside HolyArrs's category where HolyArrs may not take it. Trying again
		// changes nothing until a person acts, so the Slot waits on a decision. A Slot
		// that moved on meanwhile refuses the change; nothing is left to do then.
		_ = mediafiles.MoveSlot(ctx, m.Store, r.Job.Subject.ID, mediafiles.SlotNeedsADecision, err.Error(), domain.Actor{Kind: "job", ID: r.Job.ID})
		r.SetResult(map[string]any{"not_sent": err.Error()})
		return nil
	}
	var nd *NoDownloaderError
	if errors.As(err, &nd) {
		// Nothing can take this protocol. Trying again changes nothing until a person
		// adds a Downloader, so the Slot goes back to wanted at once with the reason,
		// never shown as sent.
		m.giveUp(ctx, r.Job.Subject.ID, int64(relID), nd.Error(), false, domain.Actor{Kind: "job", ID: r.Job.ID})
		r.SetResult(map[string]any{"not_sent": nd.Error()})
		return nil
	}
	var sg staleGrab
	if errors.As(err, &sg) {
		// The choice this Job was made for was replaced (or its title deleted): the
		// new choice has its own Job, so this one ends without touching the Slot.
		r.SetResult(map[string]any{"not_sent": sg.Error()})
		return nil
	}
	var fe interface{ Final() bool }
	if errors.As(err, &fe) && fe.Final() {
		// The Release can never be sent, so it is turned down and the Slot is searched
		// again, instead of trying five times and staying chosen.
		m.giveUp(ctx, r.Job.Subject.ID, int64(relID), err.Error(), true, domain.Actor{Kind: "job", ID: r.Job.ID})
		r.SetResult(map[string]any{"not_sent": err.Error()})
		return nil
	}
	if err != nil {
		// The last try failed on the Downloader's side (no answer, or resting), so the
		// Slot goes back to wanted and a later search can choose again. An unknown
		// outcome stays for a person.
		if r.Job.MaxAttempts > 0 && r.Job.Attempt >= r.Job.MaxAttempts && !errors.Is(err, ErrUncertain) && !errors.Is(err, ErrAttemptStopped) {
			m.giveUp(ctx, r.Job.Subject.ID, int64(relID), err.Error(), false, domain.Actor{Kind: "job", ID: r.Job.ID})
		}
		return err
	}
	r.SetResult(map[string]any{"download_id": id})
	return nil
}

// staleGrab is a grab whose choice is gone: the Slot is no longer chosen for
// the Release. Retrying never helps.
type staleGrab struct{ msg string }

func (e staleGrab) Error() string { return e.msg }

func (staleGrab) Final() bool { return true }

func (e releaseRefused) Error() string { return e.msg }

func (releaseRefused) Final() bool { return true }

func (g grabFailed) Error() string {
	s := make([]string, len(g))
	for i, e := range g {
		s[i] = e.Error()
	}
	return strings.Join(s, "; ")
}

func (g grabFailed) Final() bool {
	for _, e := range g {
		var rr releaseRefused
		if !errors.As(e, &rr) {
			return false
		}
	}
	return len(g) > 0
}

// Grab sends a Release to a Downloader for its protocol (by priority, equal
// ones in turn, resting ones skipped), records the Download and moves the Slot
// to downloading.
func (m *Manager) Grab(ctx context.Context, slotID, releaseID int64, by domain.Actor) (int64, error) {
	db := m.Store.Read()
	var rel domain.Release
	// A Release or Slot gone (its title was deleted) leaves nothing to send.
	if err := database.Get(ctx, db, "releases", releaseID, &rel); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return 0, staleGrab{fmt.Sprintf("release %d is gone", releaseID)}
		}
		return 0, fmt.Errorf("release %d: %w", releaseID, err)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, db, "slots", slotID, &sl); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return 0, staleGrab{fmt.Sprintf("slot %d is gone", slotID)}
		}
		return 0, fmt.Errorf("slot %d: %w", slotID, err)
	}
	// A stopped semantic pair stays terminal even after abandonment returns its
	// Slot to a state where a person can choose another Release.
	attempt, err := unfinishedAttempt(ctx, db, releaseID, slotID)
	if err != nil {
		return 0, err
	}
	if attempt != nil && attempt.State == "abandoned" {
		return 0, ErrAttemptStopped
	}
	if sl.State != mediafiles.SlotChosen {
		if sl.ActiveDownloadID != nil && sl.State == mediafiles.SlotDownloading {
			var prior domain.Download
			if err := database.Get(ctx, db, "downloads", *sl.ActiveDownloadID, &prior); err == nil && prior.ReleaseID != nil && *prior.ReleaseID == releaseID && slices.Contains(prior.SlotIDs, slotID) && isActive(prior.State) {
				return prior.ID, nil
			}
		}
		return 0, staleGrab{fmt.Sprintf("slot %d is %s, not chosen", slotID, sl.State)}
	}
	if sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != releaseID {
		return 0, staleGrab{"the Slot is no longer chosen for this Release"}
	}
	all, err := database.Select[domain.Downloader](ctx, db, "downloaders", "protocol = ? AND removed = 0 ORDER BY priority, id", rel.Protocol)
	if err != nil {
		return 0, err
	}

	hash := ""
	if rel.InfoHash != nil && releasename.ValidInfoHash(strings.ToLower(*rel.InfoHash)) {
		hash = strings.ToLower(*rel.InfoHash)
	}
	now := m.now()
	// Unknown outcomes pin the original Downloader before ordering or back-off. An
	// empty lookup after a restart never permits a failover append. A retry after a
	// kill finds the Download the first try wrote, or the torrent already in the
	// Downloader, and must not add a second. A finished Download that already went
	// through an Import is an old one (the same Release chosen again), so it is sent
	// again, not waited on.
	var dl *domain.Download
	if prior, err := database.Select[domain.Download](ctx, db, "downloads",
		"release_id = ? AND state IN ("+activeStates+", 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?)"+
			" AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = downloads.id AND (i.state = 'rejected' OR downloads.state = 'finished')) ORDER BY id DESC LIMIT 1",
		rel.ID, slotID); err != nil {
		return 0, err
	} else if len(prior) == 1 {
		dl = &prior[0]
	}
	// A torrent season pack for a show that takes packs a few episodes at a time
	// goes only to a qBittorrent HolyArrs may change, added stopped; with none,
	// it is fetched whole as before.
	window := 0
	if dl == nil {
		window = packWindow(ctx, db, rel, sl.ItemID)
	}
	if attempt != nil && dl == nil {
		// A send pinned to a Downloader that was removed can never be confirmed:
		// close it and pick from the Downloaders there are now.
		var gone bool
		if err := db.QueryRowContext(ctx, "SELECT removed FROM downloaders WHERE id = ?", attempt.DownloaderID).Scan(&gone); err == nil && gone {
			if err := m.resolveAttempt(ctx, attempt.ID); err != nil {
				return 0, errors.Join(ErrUncertain, err)
			}
			attempt = nil
		}
	}
	var d domain.Downloader
	if dl != nil {
		if err := database.Get(ctx, db, "downloaders", dl.DownloaderID, &d); err != nil {
			return 0, err
		}
	} else {
		var dls []domain.Downloader
		var skipped []string
		if attempt != nil {
			window = 0
			var original domain.Downloader
			if err := database.Get(ctx, db, "downloaders", attempt.DownloaderID, &original); err != nil {
				return 0, errors.Join(ErrUncertain, err)
			}

			original.Category = attempt.category
			dls = []domain.Downloader{original}
		} else {
			// Tags apply on a fresh pick only, since a send already on record keeps its
			// Downloader: use the Downloaders sharing a tag with the title, else the
			// untagged ones.
			if len(all) == 0 {
				return 0, &NoDownloaderError{Protocol: rel.Protocol}
			}
			titleTags, kind, err := slotTitleTags(ctx, db, sl.ItemID)
			if err != nil {
				return 0, err
			}
			fit := mediafiles.Downloaders(all, titleTags, func(d domain.Downloader) []string { return d.Tags })
			if len(fit) == 0 && len(all) > 0 {
				return 0, &TagsError{Protocol: rel.Protocol, Title: titleTags}
			}
			if q := packDownloaders(fit); window > 0 && len(q) > 0 {
				fit = q
			} else {
				window = 0
			}
			dls, skipped = m.order(fit, now)
			// Each Downloader sends with its category for the title's kind.
			for i := range dls {
				dls[i].Category = dls[i].CategoryFor(kind)
			}
		}
		if len(dls) == 0 {
			if len(skipped) > 0 {
				return 0, fmt.Errorf("no %s Downloader can take it now: %s", rel.Protocol, strings.Join(skipped, "; "))
			}
			return 0, fmt.Errorf("no Downloader for %s", rel.Protocol)
		}
		// When one cannot take the Release, the next of the same protocol is tried
		// (setting try_next_downloader, default on), and the error says why each one
		// failed.
		tryNext := true
		var v string
		if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'what_to_pick' AND key = 'try_next_downloader'").Scan(&v) == nil && v == "false" {
			tryNext = false
		}
		var fails []error
		for i := 0; dl == nil && i < len(dls); i++ {
			if i > 0 && !tryNext {
				break
			}
			d = dls[i]
			ext, own, err := m.send(ctx, d, rel, slotID, hash, window > 0)
			if err != nil {
				if errors.Is(err, ErrUncertain) || errors.Is(err, ErrAttemptStopped) || errors.Is(err, ErrOutsideTorrent) {
					return 0, err
				}
				fails = append(fails, err)
				continue
			}
			dl = &domain.Download{DownloaderID: d.ID, ReleaseID: &rel.ID, SlotIDs: []int64{slotID}, ExternalID: ext, State: "sent",
				FilesSeen: []domain.SeenFile{}, SavePath: m.SavePath, StartedBy: by, ProgressAt: &now}
			if window > 0 && own {
				dl.PackFiles = []domain.PackFile{}
			}
		}
		if dl == nil {
			return 0, grabFailed(fails)
		}
	}
	// The Download row and the Slot move are one write, so a kill cannot leave a
	// Download with the Slot still chosen.
	moved := []int64{slotID}
	adopted := false
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		// Another caller can reconcile an accepted send before the original caller
		// returns. Adopt that committed pair instead of resetting it.
		if dl.ID == 0 {
			prior, err := database.Select[domain.Download](ctx, tx, "downloads", `downloader_id = ? AND external_id = ? AND release_id = ?
				AND state IN (`+activeStates+`, 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?)
				AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = downloads.id AND (i.state = 'rejected' OR downloads.state = 'finished')) LIMIT 1`, d.ID, dl.ExternalID, rel.ID, slotID)
			if err != nil {
				return err
			}
			if len(prior) == 1 {
				var active sql.NullInt64
				var state string
				if err := tx.QueryRowContext(ctx, "SELECT active_download_id, state FROM slots WHERE id = ?", slotID).Scan(&active, &state); err != nil {
					return err
				}
				if active.Valid && active.Int64 == prior[0].ID && state == string(mediafiles.SlotDownloading) {
					dl = &prior[0]
					adopted = true
					return nil
				}
			}
		}
		// Resolve the intent in this same transaction as the Download and Slot. If an
		// owner abandoned it while the request was in flight, roll back.
		var intentID int64
		if journalAdapter(d.Adapter) {
			a, err := unfinishedAttempt(ctx, tx, rel.ID, slotID)
			if err != nil {
				return err
			}
			if a == nil && dl.ID == 0 {
				// A changed Release or Version snapshot cannot adopt the response of the
				// original send by reusing its numeric id.
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
		var current mediafiles.Slot
		if err := database.Get(ctx, tx, "slots", slotID, &current); err != nil {
			return err
		}
		if current.State != mediafiles.SlotChosen || current.ChosenReleaseID == nil || *current.ChosenReleaseID != rel.ID ||
			!current.CreatedAt.Equal(sl.CreatedAt) || current.VersionID != sl.VersionID {
			return errors.New("the Slot changed while sending this Release")
		}
		// A season pack or multi-episode Release chosen for several Slots is one
		// Download for all of them. Only Slots still chosen for this same Release count,
		// since an old pick list can name a Slot chosen since for another one. Only
		// Slots of the grabbed Slot's Version count, so one Download never fills two
		// Versions (the feed check may choose one Release for both).
		if many := sources.PickSlots(rel); len(many) > 1 && slices.Contains(many, slotID) {
			var version int64
			if err := tx.QueryRow("SELECT version_id FROM slots WHERE id = ?", slotID).Scan(&version); err != nil {
				return err
			}
			for _, id := range many {
				var st string
				var chosenFor, versionOf sql.NullInt64
				// A late response may only attach the identities its intent covered, even when
				// SQLite has reused a Slot's numeric id.
				if id == slotID || tx.QueryRow(`SELECT state, chosen_release_id, version_id FROM slots s WHERE id = ?
					AND (? = 0 OR EXISTS (SELECT 1 FROM downloader_attempts a, json_each(a.slot_snapshots) j
					WHERE a.id = ? AND json_extract(j.value, '$.id') = s.id AND json_extract(j.value, '$.created_at') = s.created_at))`, id, intentID, intentID).Scan(&st, &chosenFor, &versionOf) != nil ||
					st != string(mediafiles.SlotChosen) || chosenFor.Int64 != rel.ID || versionOf.Int64 != version {
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
			// The Downloader may return the id of an old Download that was not reused (its
			// Import was rejected, or the same torrent is sent again), so that row starts
			// afresh. A row still active, followed by another Slot or waiting on a removal
			// is never taken over.
			old, inUse, err := heldBy(ctx, tx, dl.DownloaderID, dl.ExternalID)
			if err != nil {
				return err
			}
			if inUse {
				return fmt.Errorf("%s gave the id of Download %d, which is still in use", d.Name, old)
			}
			if old > 0 {
				dl.ID = old
				if err := updateReused(tx, dl, by); err != nil {
					return err
				}
			} else if _, err := database.Insert(tx, "downloads", dl, by); err != nil {
				return err
			}
		}
		for _, id := range moved {
			if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, id); err != nil {
				return err
			}
			if err := mediafiles.MoveSlotTx(tx, id, mediafiles.SlotDownloading, "sent "+rel.Name+" to "+d.Name, by); err != nil {
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
		m.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotDownloading})
	}
	m.publish("download", dl)
	if window > 0 && dl.PackFiles != nil {
		m.packPending(ctx, d, *dl) // only a torrent HolyArrs added, never another app's
	}
	m.Poke()
	m.Kick(d.ID)
	return dl.ID, nil
}

// giveUp frees a Slot whose grab ended without a send: back to wanted, so a
// search can choose again. turnDown also turns the Release down, since it can
// never be sent, and searches the Slot again at once. A Slot that moved on
// meanwhile is left as it is.
func (m *Manager) giveUp(ctx context.Context, slotID, releaseID int64, reason string, turnDown bool, by domain.Actor) {
	freed := false
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE slots SET chosen_release_id = NULL WHERE id = ? AND state = 'chosen' AND chosen_release_id = ?", slotID, releaseID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}
		if turnDown {
			if _, err := tx.ExecContext(ctx, `UPDATE releases SET state = 'turned_down', hold_until = NULL,
				verdict = json_set(COALESCE(verdict, '{}'), '$.turned_down_reason', ?), updated_at = ? WHERE id = ?`,
				"could not be sent: "+reason, database.FormatTime(m.now()), releaseID); err != nil {
				return err
			}
		}
		freed = true
		return mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotWanted, "not sent: "+reason, by)
	})
	if err != nil || !freed {
		return
	}
	m.publish("slot", map[string]any{"id": slotID, "state": mediafiles.SlotWanted, "reason": reason})
	m.notSentEvent(ctx, slotID, releaseID, reason, by)
	if turnDown && m.OnFailed != nil {
		m.OnFailed(ctx, slotID)
	}
}

// slotTitleTags reads the tags and kind of the title a Slot's Item belongs to.
func slotTitleTags(ctx context.Context, db database.Querier, itemID int64) ([]string, string, error) {
	var raw, kind string
	err := db.QueryRowContext(ctx, "SELECT t.tags, t.kind FROM items i JOIN titles t ON t.id = i.title_id WHERE i.id = ?", itemID).Scan(&raw, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var tags []string
	_ = json.Unmarshal([]byte(raw), &tags)
	return tags, kind, nil
}

// titlePackWindow reads pack_window of the title an Item belongs to.
func titlePackWindow(ctx context.Context, db database.Querier, itemID int64) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT t.pack_window FROM items i JOIN titles t ON t.id = i.title_id WHERE i.id = ? AND t.kind = 'show'", itemID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// packEp is one episode of a pack's season with its Slot in the Version.
type packEp struct {
	ref    mediafiles.EpisodeRef
	slotID int64
}

// packEpisodes lists the episodes of the season a Slot belongs to, with their

// Slots in its Version, and the show's episode order.
func packEpisodes(ctx context.Context, db *sql.DB, slotID int64) ([]packEp, string, error) {
	var titleID, versionID int64
	var season sql.NullInt64
	var order string
	if err := db.QueryRowContext(ctx, `SELECT i.title_id, s.version_id, json_extract(i.numbers, '$.season'), COALESCE(t.episode_order, '')
		FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE s.id = ?`, slotID).Scan(&titleID, &versionID, &season, &order); err != nil {
		return nil, "", err
	}
	if order == "" {
		order = mediafiles.OrderAired
	}
	rows, err := db.QueryContext(ctx, `SELECT i.id, COALESCE(json_extract(i.numbers, '$.episode'), 0), COALESCE(json_extract(i.numbers, '$.absolute'), 0),
		COALESCE(i.air_day, ''), COALESCE(s.id, 0) FROM items i LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = ?
		WHERE i.title_id = ? AND i.kind = 'episode' AND i.dropped_at IS NULL AND json_extract(i.numbers, '$.season') IS ?`, versionID, titleID, season)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []packEp
	for rows.Next() {
		e := packEp{ref: mediafiles.EpisodeRef{Season: int(season.Int64)}}
		if err := rows.Scan(&e.ref.ItemID, &e.ref.Episode, &e.ref.Absolute, &e.ref.AirDay, &e.slotID); err != nil {
			return nil, "", err
		}
		out = append(out, e)
	}
	return out, order, rows.Err()
}

// savePackFiles writes a Download's pack_files; nil writes NULL, a normal
// download again.
func savePackFiles(ctx context.Context, s *database.Store, id int64, files []domain.PackFile) error {
	var v any
	if files != nil {
		b, _ := json.Marshal(files)
		v = string(b)
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE downloads SET pack_files = ? WHERE id = ?", v, id)
		return err
	})
}

// packSlotsWanted says every Slot of a pack file is wanted and its episode

// and season are kept up to date.
func packSlotsWanted(ctx context.Context, db *sql.DB, slotIDs []int64) (bool, error) {
	if len(slotIDs) == 0 {
		return false, nil
	}
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM slots s JOIN items i ON i.id = s.item_id LEFT JOIN parts p ON p.id = i.part_id
		WHERE s.id IN (SELECT value FROM json_each(?)) AND s.state = 'wanted' AND i.keep_up_to_date = 1 AND (p.id IS NULL OR p.keep_up_to_date = 1)`, jsonIDs(slotIDs)).Scan(&n)
	return n == len(slotIDs), err
}

// widenPack records switched-on pack files in one write: the new Slots move

// to downloading for this Download, and the Download goes back to

// downloading.
func widenPack(ctx context.Context, s *database.Store, d *domain.Download, slots []int64, now time.Time, by domain.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range slots {
			if _, err := tx.ExecContext(ctx, "UPDATE slots SET chosen_release_id = ? WHERE id = ?", *d.ReleaseID, id); err != nil {
				return err
			}
			for _, to := range []mediafiles.SlotState{mediafiles.SlotSearching, mediafiles.SlotChosen, mediafiles.SlotDownloading} {
				if err := mediafiles.MoveSlotTx(tx, id, to, "the next episode of the season pack", by); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, "UPDATE slots SET active_download_id = ? WHERE id = ?", d.ID, id); err != nil {
				return err
			}
		}
		b, _ := json.Marshal(d.PackFiles)
		at := database.FormatTime(now)
		if _, err := tx.ExecContext(ctx, `UPDATE downloads SET pack_files = ?, slot_ids = ?, state = CASE WHEN state IN (`+activeStates+`) THEN state ELSE 'downloading' END,
			progress = 0, progress_at = ?, finished_at = NULL, state_reason = NULL, updated_at = ? WHERE id = ?`, string(b), jsonIDs(d.SlotIDs), at, at, d.ID); err != nil {
			return err
		}
		return database.Get(ctx, tx, "downloads", d.ID, d)
	})
}

// claimPack swaps a Download's pack_files from was to next in one write, and
// fails with ErrPackChanged when the stored list is no longer was. Widen
// records its choice this way before it changes the Downloader, and swaps
// back when the Downloader refuses.
func claimPack(ctx context.Context, s *database.Store, id int64, was, next []domain.PackFile) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		var cur domain.Download
		if err := database.Get(ctx, tx, "downloads", id, &cur); err != nil {
			return err
		}
		a, _ := json.Marshal(cur.PackFiles)
		b, _ := json.Marshal(was)
		if string(a) != string(b) {
			return ErrPackChanged
		}
		n, _ := json.Marshal(next)
		_, err := tx.ExecContext(ctx, "UPDATE downloads SET pack_files = ? WHERE id = ?", string(n), id)
		return err
	})
}

// partialPacks lists a show's season packs fetched a few episodes at a time

// that are still in the Downloader (newest first), for one season or every

// season when season is 0, with the show's pack_window.
func partialPacks(ctx context.Context, db *sql.DB, titleID int64, season int) ([]domain.Download, int, error) {
	var window int
	if err := db.QueryRowContext(ctx, "SELECT pack_window FROM titles WHERE id = ?", titleID).Scan(&window); err != nil {
		return nil, 0, err
	}
	ds, err := database.Select[domain.Download](ctx, db, "downloads", `json_array_length(pack_files) > 0 AND state IN (`+activeStates+`, 'finished', 'seeding')
		AND EXISTS (SELECT 1 FROM json_each(pack_files) f, json_each(json_extract(f.value, '$.items')) it JOIN items i ON i.id = it.value
		WHERE i.title_id = ? AND (? = 0 OR json_extract(i.numbers, '$.season') = ?)) ORDER BY id DESC`, titleID, season, season)
	return ds, window, err
}

// PackTitles lists the shows that take season packs a few episodes at a
// time and have such a pack still in the Downloader.
func PackTitles(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id FROM titles t WHERE t.pack_window > 0 AND EXISTS (SELECT 1 FROM downloads d,
		json_each(d.pack_files) f, json_each(json_extract(f.value, '$.items')) it JOIN items i ON i.id = it.value
		WHERE json_array_length(d.pack_files) > 0 AND d.state IN (`+activeStates+`, 'finished', 'seeding') AND i.title_id = t.id) ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// settleLostChoices frees chosen Slots that nothing will ever send: no grab Job
// waits for the choice, no Download follows it and no send to a Downloader is
// pending. Pending sends to a Downloader that was removed are closed first, so
// their Slots are freed too. Each goes back to wanted with the reason.
func (m *Manager) settleLostChoices(ctx context.Context) {
	now := m.now()
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE downloader_attempts SET state = 'resolved', updated_at = ?
			WHERE state = 'pending' AND downloader_id IN (SELECT id FROM downloaders WHERE removed = 1)`, now.Format(time.RFC3339Nano))
		return err
	}); err != nil {
		log.Printf("downloads: could not close sends to removed Downloaders: %v", err)
	}
	rows, err := m.Store.Read().QueryContext(ctx, `SELECT s.id, s.chosen_release_id FROM slots s
		WHERE s.state = 'chosen' AND s.chosen_release_id IS NOT NULL AND s.active_download_id IS NULL AND s.updated_at <= ?
		AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.kind = 'grab' AND j.state IN ('queued', 'running')
			AND json_extract(j.subject, '$.id') = s.id AND json_extract(j.params, '$.release_id') = s.chosen_release_id)
		AND NOT EXISTS (SELECT 1 FROM downloader_attempts a WHERE a.state = 'pending' AND a.release_id = s.chosen_release_id
			AND EXISTS (SELECT 1 FROM json_each(a.slot_snapshots) x WHERE json_extract(x.value, '$.id') = s.id))
		AND NOT EXISTS (SELECT 1 FROM downloads d WHERE d.release_id = s.chosen_release_id AND d.state IN (`+activeStates+`)
			AND EXISTS (SELECT 1 FROM json_each(d.slot_ids) x WHERE x.value = s.id))
		ORDER BY s.id LIMIT 200`, database.FormatTime(now.Add(-lostChoiceWait)))
	if err != nil {
		log.Printf("downloads: could not read chosen Slots: %v", err)
		return
	}
	var lost [][2]int64
	for rows.Next() {
		var p [2]int64
		if rows.Scan(&p[0], &p[1]) == nil {
			lost = append(lost, p)
		}
	}
	rows.Close()
	for _, p := range lost {
		log.Printf("downloads: warning: Release %d chosen for Slot %d was never sent to a Downloader; the Slot goes back to wanted", p[1], p[0])
		m.giveUp(ctx, p[0], p[1], lostChoiceReason, false, domain.System)
	}
}
