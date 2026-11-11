package downloads

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

var (
	ErrPendingAttempt = errors.New("this Downloader has an unknown send outcome; reconcile or abandon it before editing or removing the Downloader")
	ErrAttemptStopped = errors.New("this send was abandoned; inspect the Downloader before choosing a different Release")
)

// Attempt is the public view of a send record. It never shows the correlation
// marker, the address or any credential.
type Attempt struct {
	ID           int64  `json:"id"`
	DownloaderID int64  `json:"downloader_id"`
	ReleaseID    int64  `json:"release_id"`
	SlotID       int64  `json:"slot_id"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type sendAttempt struct {
	Attempt
	marker, category, expectedID string
	created                      bool
}

// Attempts lists kept send outcomes, without secrets or raw remote errors.
func (m *Manager) Attempts(ctx context.Context, downloaderID int64) ([]Attempt, error) {
	var d domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", downloaderID, &d); err != nil {
		return nil, err
	}
	rows, err := m.Store.Read().QueryContext(ctx, `SELECT id, downloader_id, release_id, slot_id, state, created_at, updated_at
		FROM downloader_attempts WHERE downloader_id = ? AND (state = 'pending' OR id IN
		(SELECT id FROM downloader_attempts WHERE downloader_id = ? AND state <> 'pending' ORDER BY id DESC LIMIT 100))
		ORDER BY id DESC`, downloaderID, downloaderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		var a Attempt
		if err := rows.Scan(&a.ID, &a.DownloaderID, &a.ReleaseID, &a.SlotID, &a.State, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AbandonAttempt stops all later sends for this Release and Version and hands
// its still-current chosen Slots back for a person to decide. It does not cancel
// or remove the request already made on the Downloader, nor prove it is absent.
func (m *Manager) AbandonAttempt(ctx context.Context, downloaderID, attemptID int64, by domain.Actor) error {
	var moved []int64
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var state string
		if err := tx.QueryRowContext(ctx, "SELECT state FROM downloader_attempts WHERE id = ? AND downloader_id = ?", attemptID, downloaderID).Scan(&state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return database.ErrNotFound
			}
			return err
		}
		if state == "abandoned" {
			return nil
		}
		if state != "pending" {
			return ErrAttemptStopped
		}
		actor, err := json.Marshal(by)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE downloader_attempts SET state = 'abandoned', abandoned_by = ?, updated_at = ? WHERE id = ? AND state = 'pending'", string(actor), m.now().Format(time.RFC3339Nano), attemptID); err != nil {
			return err
		}
		// Stored numeric ids are not identities. Only the original Release, Version
		// and Slot snapshots may leave chosen, and only while still chosen for this
		// Release. The transaction keeps this check and the recovery change atomic.
		rows, err := tx.QueryContext(ctx, `SELECT s.id, s.created_at, s.version_id, a.release_id
			FROM downloader_attempts a JOIN releases r ON r.id = a.release_id AND r.created_at = a.release_created_at
			JOIN versions v ON v.id = a.version_id AND v.created_at = a.version_created_at
			JOIN slots s ON s.version_id = v.id AND s.state = 'chosen' AND s.chosen_release_id = r.id
			WHERE a.id = ? AND EXISTS (SELECT 1 FROM json_each(a.slot_snapshots) j
			WHERE json_extract(j.value, '$.id') = s.id AND json_extract(j.value, '$.created_at') = s.created_at)`, attemptID)
		if err != nil {
			return err
		}
		type choice struct {
			id, versionID, releaseID int64
			created                  string
		}
		var choices []choice
		for rows.Next() {
			var c choice
			if err := rows.Scan(&c.id, &c.created, &c.versionID, &c.releaseID); err != nil {
				rows.Close()
				return err
			}
			choices = append(choices, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, c := range choices {
			res, err := tx.ExecContext(ctx, `UPDATE slots SET chosen_release_id = NULL WHERE id = ? AND created_at = ?
				AND version_id = ? AND state = 'chosen' AND chosen_release_id = ?`, c.id, c.created, c.versionID, c.releaseID)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n == 0 {
				continue
			}
			if err := mediafiles.MoveSlotTx(tx, c.id, mediafiles.SlotNeedsADecision, "send abandoned; inspect the Downloader before choosing another Release", by); err != nil {
				return err
			}
			moved = append(moved, c.id)
		}
		return nil
	})
	if err == nil {
		for _, id := range moved {
			m.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotNeedsADecision})
		}
	}
	return err
}

func unfinishedAttempt(ctx context.Context, q database.Querier, releaseID, slotID int64) (*sendAttempt, error) {
	a := &sendAttempt{}
	err := q.QueryRowContext(ctx, `SELECT a.id, a.downloader_id, a.release_id, a.slot_id, a.state, a.created_at, a.updated_at, a.marker, a.category, a.expected_id
		FROM downloader_attempts a JOIN releases r ON r.id = a.release_id AND r.created_at = a.release_created_at
		JOIN versions v ON v.id = a.version_id AND v.created_at = a.version_created_at
		JOIN slots s ON s.version_id = v.id AND s.id = ?
		WHERE a.release_id = ? AND a.state IN ('pending', 'abandoned')
		AND EXISTS (SELECT 1 FROM json_each(a.slot_snapshots) j WHERE json_extract(j.value, '$.id') = s.id AND json_extract(j.value, '$.created_at') = s.created_at)`, slotID, releaseID).
		Scan(&a.ID, &a.DownloaderID, &a.ReleaseID, &a.SlotID, &a.State, &a.CreatedAt, &a.UpdatedAt, &a.marker, &a.category, &a.expectedID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

// claimAttempt serializes creating an intent with settings changes. Only the
// creator may append; any later caller can only reconcile the same marker.
func (m *Manager) claimAttempt(ctx context.Context, d domain.Downloader, releaseID, slotID int64) (*sendAttempt, error) {
	var a *sendAttempt
	err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var err error
		a, err = unfinishedAttempt(ctx, tx, releaseID, slotID)
		if err != nil {
			return err
		}
		if a != nil {
			if a.State == "abandoned" {
				return ErrAttemptStopped
			}
			if a.DownloaderID != d.ID {
				return ErrUncertain
			}
			var current mediafiles.Slot
			if err := database.Get(ctx, tx, "slots", slotID, &current); err != nil {
				return err
			}
			if current.State != mediafiles.SlotChosen || current.ChosenReleaseID == nil || *current.ChosenReleaseID != releaseID {
				return errors.New("the Slot is no longer chosen for this Release")
			}
			return nil
		}
		if !journalAdapter(d.Adapter) {
			return nil
		}
		// Another Grab may have committed while this caller was picking a Downloader.
		// Reuse its recorded identity without appending.
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT external_id FROM downloads WHERE downloader_id = ? AND release_id = ?
			AND state IN (`+activeStates+`, 'finished') AND EXISTS (SELECT 1 FROM json_each(slot_ids) WHERE value = ?)
			AND NOT EXISTS (SELECT 1 FROM imports i WHERE i.download_id = downloads.id AND (i.state = 'rejected' OR downloads.state = 'finished')) LIMIT 1`, d.ID, releaseID, slotID).Scan(&existing); err == nil {
			a = &sendAttempt{Attempt: Attempt{DownloaderID: d.ID, ReleaseID: releaseID, SlotID: slotID, State: "resolved"}, marker: existing}
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var currentSlot mediafiles.Slot
		if err := database.Get(ctx, tx, "slots", slotID, &currentSlot); err != nil {
			return err
		}
		if currentSlot.State != mediafiles.SlotChosen || currentSlot.ChosenReleaseID == nil || *currentSlot.ChosenReleaseID != releaseID {
			return errors.New("the Slot is no longer chosen for this Release")
		}
		var current domain.Downloader
		if err := database.Get(ctx, tx, "downloaders", d.ID, &current); err != nil {
			return err
		}
		sameSecret := d.SecretRef == nil && current.SecretRef == nil || d.SecretRef != nil && current.SecretRef != nil && *d.SecretRef == *current.SecretRef
		if current.Removed || current.Adapter != d.Adapter || current.URL != d.URL || current.Username != d.Username ||
			!slices.Contains(current.Categories(), d.Category) || current.AllowLocal != d.AllowLocal || !sameSecret {
			return errors.New("the Downloader changed before sending; try again")
		}
		var nonce [24]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return err
		}
		now := m.now().Format(time.RFC3339Nano)
		var releaseCreated, slotCreated, versionCreated, link string
		var versionID int64
		var hash sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT r.created_at, s.created_at, r.info_hash, r.link_ref, v.id, v.created_at
			FROM releases r CROSS JOIN slots s JOIN versions v ON v.id = s.version_id WHERE r.id = ? AND s.id = ?`, releaseID, slotID).
			Scan(&releaseCreated, &slotCreated, &hash, &link, &versionID, &versionCreated); err != nil {
			return err
		}
		// A kept pack intent can outlive one of its Slots. A reused numeric id must
		// not adopt that intent or start a new send.
		var priorState string
		if err := tx.QueryRowContext(ctx, `SELECT state FROM downloader_attempts WHERE release_id = ? AND release_created_at = ?
			AND version_id = ? AND version_created_at = ? AND state IN ('pending', 'abandoned')`, releaseID, releaseCreated, versionID, versionCreated).Scan(&priorState); err == nil {
			if priorState == "abandoned" {
				return ErrAttemptStopped
			}
			return ErrUncertain
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var rel domain.Release
		if err := database.Get(ctx, tx, "releases", releaseID, &rel); err != nil {
			return err
		}
		covered := []int64{slotID}
		if many := sources.PickSlots(rel); slices.Contains(many, slotID) {
			covered = many
		}
		type slotSnapshot struct {
			ID        int64  `json:"id"`
			CreatedAt string `json:"created_at"`
		}
		snapshots := []slotSnapshot{}
		for _, id := range covered {
			var created string
			if err := tx.QueryRowContext(ctx, "SELECT created_at FROM slots WHERE id = ? AND version_id = ?", id, versionID).Scan(&created); err == nil {
				snapshots = append(snapshots, slotSnapshot{ID: id, CreatedAt: created})
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		snapshotJSON, err := json.Marshal(snapshots)
		if err != nil {
			return err
		}
		expected := ""
		if d.Protocol == "torrent" {
			if releasename.ValidInfoHash(strings.ToLower(hash.String)) {
				expected = strings.ToLower(hash.String)
			} else if h, ok := magnetHash(link); ok {
				expected = h
			}
		}
		a = &sendAttempt{Attempt: Attempt{DownloaderID: d.ID, ReleaseID: releaseID, SlotID: slotID, State: "pending", CreatedAt: now, UpdatedAt: now},
			marker: "nzbget-" + hex.EncodeToString(nonce[:]), category: d.Category, expectedID: expected, created: true}
		res, err := tx.ExecContext(ctx, `INSERT INTO downloader_attempts(downloader_id, release_id, slot_id, release_created_at, slot_created_at, version_id, version_created_at, slot_snapshots, marker, expected_id, category, state, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`, d.ID, releaseID, slotID, releaseCreated, slotCreated, versionID, versionCreated, string(snapshotJSON), a.marker, expected, a.category, now, now)
		if err != nil {
			return err
		}
		a.ID, err = res.LastInsertId()
		return err
	})
	return a, err
}

func journalAdapter(adapter string) bool {
	return adapter == "nzbget" || adapter == "transmission" || adapter == "deluge" || adapter == "rtorrent"
}

func pendingAttemptCount(tx *sql.Tx, downloaderID int64) (int, error) {
	var n int
	err := tx.QueryRow("SELECT count(*) FROM downloader_attempts WHERE downloader_id = ? AND state = 'pending'", downloaderID).Scan(&n)
	return n, err
}
