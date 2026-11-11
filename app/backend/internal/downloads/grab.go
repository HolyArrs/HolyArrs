package downloads

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	id, err := m.Grab(ctx, r.Job.Subject.ID, int64(relID), domain.Actor{Kind: "job", ID: r.Job.ID})
	var te *TagsError
	if errors.As(err, &te) {
		// No Downloader takes the title's tags. Trying again changes nothing until a
		// person changes tags, so the Slot waits on a decision. A Slot that moved on
		// meanwhile refuses the change; nothing is left to do then.
		_ = mediafiles.MoveSlot(ctx, m.Store, r.Job.Subject.ID, mediafiles.SlotNeedsADecision, te.Error(), domain.Actor{Kind: "job", ID: r.Job.ID})
		r.SetResult(map[string]any{"not_sent": te.Error()})
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
	if err := database.Get(ctx, db, "releases", releaseID, &rel); err != nil {
		return 0, fmt.Errorf("release %d: %w", releaseID, err)
	}
	var sl mediafiles.Slot
	if err := database.Get(ctx, db, "slots", slotID, &sl); err != nil {
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
		return 0, fmt.Errorf("slot %d is %s, not chosen", slotID, sl.State)
	}
	if sl.ChosenReleaseID == nil || *sl.ChosenReleaseID != releaseID {
		return 0, errors.New("the Slot is no longer chosen for this Release")
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
	var d domain.Downloader
	if dl != nil {
		if err := database.Get(ctx, db, "downloaders", dl.DownloaderID, &d); err != nil {
			return 0, err
		}
	} else {
		var dls []domain.Downloader
		var skipped []string
		if attempt != nil {
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
			titleTags, kind, err := slotTitleTags(ctx, db, sl.ItemID)
			if err != nil {
				return 0, err
			}
			fit := mediafiles.Downloaders(all, titleTags, func(d domain.Downloader) []string { return d.Tags })
			if len(fit) == 0 && len(all) > 0 {
				return 0, &TagsError{Protocol: rel.Protocol, Title: titleTags}
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
			ext, err := m.send(ctx, d, rel, slotID, hash)
			if err != nil {
				if errors.Is(err, ErrUncertain) || errors.Is(err, ErrAttemptStopped) {
					return 0, err
				}
				fails = append(fails, err)
				continue
			}
			dl = &domain.Download{DownloaderID: d.ID, ReleaseID: &rel.ID, SlotIDs: []int64{slotID}, ExternalID: ext, State: "sent",
				FilesSeen: []domain.SeenFile{}, SavePath: m.SavePath, StartedBy: by, ProgressAt: &now}
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
			// Import was rejected), so that row starts afresh. A row another Slot still
			// follows is never taken over.
			var old int64
			var oldState string
			if tx.QueryRow("SELECT id, state FROM downloads WHERE downloader_id = ? AND external_id = ?", dl.DownloaderID, dl.ExternalID).Scan(&old, &oldState) == nil {
				if isActive(oldState) {
					return fmt.Errorf("%s gave the id of Download %d, which is still active", d.Name, old)
				}
				dl.ID = old
				if err := database.Update(tx, "downloads", dl, by); err != nil {
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
	if turnDown && m.OnFailed != nil {
		m.OnFailed(ctx, slotID)
	}
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
			// A definite refusal before the append may be retried. A failed cleanup is
			// treated conservatively: keep the intent and suppress failover.
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
	// Download row holds its id. A failed or vanished one must not come back, and a
	// row for another Release or Slot is not this one.
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
		// A recovery is only a lookup. It needs neither the original Source nor a
		// send-spacing reservation, and may never fetch or append.
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
			if skip[j.ID] || (d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "rtorrent") && j.Category != d.Category {
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
	var src domain.Source
	if err := database.Get(ctx, m.Store.Read(), "sources", rel.SourceID, &src); err != nil {
		return "", errors.New("the Release's Source could not be read")
	}
	allowLocal := src.AllowLocal
	if d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "rtorrent" || d.Adapter == "nzbget" {
		if !strings.HasPrefix(link, "magnet:") && src.ProxyURL != "" {
			return "", errors.New("this Downloader cannot yet fetch Release files through a Source proxy")
		}
	}
	if link, err = grabLinkAllowed(d.Adapter, link, allowLocal); err != nil {
		return "", err
	}
	// qBittorrent and SABnzbd would fetch a link themselves from inside the home
	// network, where a name leading home is never checked. Without local network
	// access HolyArrs fetches the file and hands it over; a link that redirects to a
	// magnet sends the magnet. A Source with a proxy is fetched through it, with the
	// same checks. A qBittorrent Release with no info hash at all (an http link to a
	// .torrent, as private sites through Jackett or Prowlarr give) is fetched the
	// same way, even from a local Source: the hash comes from the file, so the
	// download can be followed.
	var file []byte
	noHash := d.Adapter == "qbittorrent" && hash == "" && linkHash(link) == ""
	if (d.Adapter == "qbittorrent" || d.Adapter == "sabnzbd") && (!allowLocal || noHash) && !strings.HasPrefix(link, "magnet:") {
		var rt http.RoundTripper
		if src.ProxyURL != "" {
			if m.SourceTransport == nil {
				return "", errors.New("this Downloader cannot yet fetch Release files through a Source proxy")
			}
			if rt, err = m.SourceTransport(ctx, src); err != nil {
				return "", err
			}
		}
		limit := addFetchMax
		if d.Adapter == "sabnzbd" {
			limit = sabMaxNZB
		}
		var magnet string
		if file, magnet, err = fetchGrabLink(ctx, link, limit, rt, sourceLocalHost(src.URL, allowLocal)); err == nil && magnet != "" {
			link, _, err = safeMagnet(magnet, allowLocal)
		}
		if err != nil {
			return "", err
		}
	}
	// Prowlarr and Jackett answer magnet-only sites with a redirect to a magnet,
	// which Transmission, Deluge, rTorrent and some qBittorrent stand-ins cannot follow. A
	// torrent link not read above (a home Source whose Release has a hash) is read
	// here and the magnet or file is sent; if that fails the link is sent as before.
	torrentAdapter := d.Adapter == "transmission" || d.Adapter == "deluge" || d.Adapter == "rtorrent" || d.Adapter == "qbittorrent" && src.ProxyURL == ""
	if torrentAdapter && file == nil && !strings.HasPrefix(link, "magnet:") {
		link, file = prefetchTorrent(ctx, link, allowLocal, sourceLocalHost(src.URL, allowLocal))
		if hash == "" {
			hash = linkHash(link)
		}
	}
	if d.Adapter == "qbittorrent" && file != nil {
		// Some trackers print a newline after the file; qBittorrent reads past it.
		file = bytes.TrimRight(file, " \t\r\n")
		// Trackers, web seeds and DHT nodes in the file that point at home are left
		// out, and the info hash stays the same. A Source allowed on the local network
		// keeps them: its file is fetched only to learn the hash, and a tracker at home
		// is then the user's own.
		if !allowLocal {
			if file, err = safeTorrent(file); err != nil {
				return "", errors.New("the Release file is not a torrent")
			}
		}
		h, err := torrentFileHash(file)
		if err != nil {
			// Often a one-off page (a limit or a check) instead of the file, so try again
			// and do not turn the Release down.
			return "", errors.New("the Release file is not a torrent")
		}
		if hash != "" && !strings.EqualFold(hash, h) {
			return "", releaseRefused{"the torrent file does not match its info hash"}
		}
		if hash == "" {
			// The hash came from the file, so the earlier look for it in qBittorrent did
			// not run, and adding a torrent it already has would fail.
			if j, err := e.Get(ctx, h); err == nil {
				if skip[j.ID] {
					return "", errors.New("the torrent belongs to another download or category")
				}
				if j.ID != "" {
					return j.ID, nil
				}
				return h, nil
			}
		}
		hash = h
	}
	// qBittorrent answers an add without an id, so HolyArrs follows the torrent by
	// its info hash, from the Release or its magnet link. Without one the torrent
	// would download unseen, so it is not sent.
	if d.Adapter == "qbittorrent" && hash == "" {
		if hash = linkHash(link); hash == "" {
			return "", releaseRefused{"the Release has no info hash, so " + d.Name + "'s download of it could not be followed"}
		}
	}
	done, err := m.space(ctx, d.URL)
	if err != nil {
		return "", err
	}
	req := AddRequest{URL: link, File: file, Name: rel.Name, Category: d.Category, SavePath: m.SavePath, InfoHash: hash, Skip: skip,
		AllowLocal: allowLocal, OperationKey: fmt.Sprintf("%d:%d:%d", d.ID, rel.ID, slotID)}
	if a != nil {
		req.AttemptMarker, req.ReconcileOnly = a.marker, !a.created
	}
	ext, err = e.Add(ctx, req)
	done()
	if err != nil {
		// Only a Downloader that did not answer, or turned down the sign-in, rests. One
		// that answered and refused this Release (a bad link, no category) stays ready
		// for the next one.
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

func jsonIDs(ids []int64) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

// grabLinkAllowed applies the Source's network rule to a feed link before a
// Downloader gets it. Magnets are rebuilt by safeMagnet, and an address that
// qBittorrent or SABnzbd would fetch itself may not name a literal local,
// link-local or metadata host unless the Source may reach the local network.
func grabLinkAllowed(adapter, link string, allowLocal bool) (string, error) {
	if len(link) >= 7 && strings.EqualFold(link[:7], "magnet:") {
		clean, _, err := safeMagnet(link, allowLocal)
		return clean, err
	}
	if (adapter == "qbittorrent" || adapter == "sabnzbd") && !trackerAllowed(link, allowLocal) {
		return "", errors.New("the Release link points at a local or blocked address")
	}
	return link, nil
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
