package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

func (r *run) titleHere(t *TitlePlan) int64 {
	db := r.m.Store.Read()
	var id int64
	switch t.Kind {
	case "movie":
		id, _ = library.TitleByTMDB(r.ctx, db, strings.TrimPrefix(t.ID, "tmdb:"))
	case "show":
		ids := map[string]string{}
		for _, k := range []string{"tvdb", "tmdb", "imdb"} {
			if t.IDs[k] != "" {
				ids[k] = t.IDs[k]
			}
		}
		id, _ = library.ShowByIDs(r.ctx, db, ids)
	case "artist":
		id, _ = library.ArtistByMBID(r.ctx, db, strings.TrimPrefix(t.ID, "mbid:"))
	}
	return id
}

func (r *run) title(it Item) (string, string) {
	t := it.Title
	if id := r.titleHere(t); id != 0 {
		if t.Music == nil {
			return ResAlready, "already in the Library"
		}
		if r.dry {
			return ResChange, "keeps its imported artist and album choices"
		}
		err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
			before, err := captureMusicState(tx, id, t.Music)
			if err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE titles SET keep_up_to_date = ? WHERE id = ?", t.KeepUpToDate, id); err != nil {
				return err
			}
			if err = r.applyMusicPolicyTx(tx, id, t.Music); err != nil {
				return err
			}
			after, err := captureMusicState(tx, id, t.Music)
			if err != nil {
				return err
			}
			bj, _ := json.Marshal(before)
			aj, _ := json.Marshal(after)
			return r.markTx(tx, it.Key, "music_policy", id, string(bj), string(aj))
		})
		return result(err, ResChanged, "kept imported artist and album choices")
	}
	lib := filepath.FromSlash(t.Library)
	dir := filepath.Join(lib, t.Folder)
	if !filepath.IsLocal(t.Folder) || strings.ContainsAny(t.Folder, `/\`) {
		return ResFailed, "its folder name is not one folder"
	}
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return ResNotFound, "no folder at " + filepath.ToSlash(dir) + " on this server; add a path rule"
	}
	if r.dry {
		return ResAdopt, "uses the folder " + filepath.ToSlash(dir) + " where it is; no file is moved or renamed"
	}
	lfID := r.id("SELECT id FROM library_folders WHERE path = ?", filepath.Clean(lib))
	if lfID == 0 {
		return ResNotFound, "its Library folder " + t.Library + " is not here (not moved, or left out)"
	}
	c := library.AdoptChoice{Folder: t.Folder, ID: t.ID}
	if t.Kind == "movie" {
		c = library.AdoptChoice{Folder: t.Folder, TMDB: strings.TrimPrefix(t.ID, "tmdb:"), ReleaseGate: t.ReleaseGate, GateDelayDays: t.GateDelayDays}
	}
	res, err := r.m.Library.Adopt(r.ctx, lfID, []library.AdoptChoice{c}, r.by)
	if err == nil && len(res) == 1 && res[0].Error != "" {
		err = errors.New(res[0].Error)
	}
	if err != nil {
		return ResFailed, err.Error()
	}
	id := res[0].TitleID
	// Adopting runs its own steps; when the mark cannot be kept the title
	// leaves the Library again, so nothing stays that undo cannot find.
	if mk := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error { return r.markTx(tx, it.Key, "title", id, "", "") }); mk != nil {
		if _, removeErr := r.m.Library.Remove(context.WithoutCancel(r.ctx), id, library.RemoveChoice{}, true, r.by); removeErr != nil && !errors.Is(removeErr, database.ErrNotFound) && !errors.Is(removeErr, library.ErrAlreadyRemoved) {
			recoveryErr := r.m.Store.Write(context.WithoutCancel(r.ctx), func(tx *sql.Tx) error { return r.markTx(tx, it.Key, "title", id, "", "") })
			if recoveryErr == nil {
				return ResFailed, "adopted, its first undo mark failed and cleanup failed; a recovery mark was kept: " + mk.Error() + "; " + removeErr.Error()
			}
			return ResFailed, "adopted, its undo mark could not be kept, cleanup failed, and the recovery mark also failed: " + mk.Error() + "; " + removeErr.Error() + "; " + recoveryErr.Error()
		}
		return ResFailed, "adopted, but its mark was not kept, so it was taken out again: " + mk.Error()
	}
	pr := r.presetID(t.Preset, t.Kind)
	err = r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET keep_up_to_date = ? WHERE id = ?", t.KeepUpToDate, id); err != nil {
			return err
		}
		if t.Music != nil {
			if err := r.applyMusicPolicyTx(tx, id, t.Music); err != nil {
				return err
			}
		}
		if pr != 0 {
			_, err := tx.Exec("UPDATE versions SET pick_rules_id = ? WHERE title_id = ?", pr, id)
			return err
		}
		return nil
	})
	if err != nil {
		if _, removeErr := r.m.Library.Remove(context.WithoutCancel(r.ctx), id, library.RemoveChoice{}, true, r.by); removeErr != nil && !errors.Is(removeErr, database.ErrNotFound) && !errors.Is(removeErr, library.ErrAlreadyRemoved) {
			return ResFailed, "adopted, but its choices were not kept and cleanup failed; the undo mark was kept: " + err.Error() + "; " + removeErr.Error()
		}
		markErr := r.m.Store.Write(context.WithoutCancel(r.ctx), func(tx *sql.Tx) error {
			_, e := tx.Exec("DELETE FROM move_made WHERE move_id = ? AND key = ?", r.move, it.Key)
			return e
		})
		if markErr != nil {
			return ResFailed, "adopted, but its choices were not kept; cleanup removed it, but its stale mark could not be deleted and will be ignored on retry: " + err.Error() + "; " + markErr.Error()
		}
		return ResFailed, "adopted, but its choices were not kept: " + err.Error()
	}
	for _, n := range t.SeasonsOff {
		_ = shows.New(r.m.Library).SetSeason(r.ctx, id, n, false, r.by) // a season the show does not have is fine
	}
	d := "uses the folder " + filepath.ToSlash(dir) + " where it is"
	if res[0].File != "" {
		d += ", with " + res[0].File
	}
	if !t.KeepUpToDate {
		d += "; Keep up to date off"
	}
	return ResAdopted, d
}

type musicPartState struct {
	PartID            int64            `json:"part_id"`
	PartFingerprint   string           `json:"part_fingerprint"`
	Keep              bool             `json:"keep"`
	Chosen            string           `json:"chosen,omitempty"`
	ChosenBy          string           `json:"chosen_by,omitempty"`
	Tracks            map[int64]bool   `json:"tracks,omitempty"`
	TrackFingerprints map[int64]string `json:"track_fingerprints"`
}
type musicState struct {
	KeepUpToDate                          bool
	NewAlbums, Types, Secondary, Statuses string
	Parts                                 []musicPartState `json:"parts,omitempty"`
}

func captureMusicState(q interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}, titleID int64, p *MusicPolicyPlan) (musicState, error) {
	var s musicState
	if err := q.QueryRow("SELECT keep_up_to_date, new_parts_policy, music_types, music_secondary, music_statuses FROM titles WHERE id = ?", titleID).Scan(&s.KeepUpToDate, &s.NewAlbums, &s.Types, &s.Secondary, &s.Statuses); err != nil {
		return s, err
	}
	for _, a := range p.Albums {
		var x musicPartState
		if err := q.QueryRow(`SELECT id, keep_up_to_date, created_at || char(31) || title_id || char(31) || kind || char(31) || source_ids FROM parts WHERE title_id = ? AND kind = 'album' AND json_extract(source_ids, '$.musicbrainz') = ? AND dropped_at IS NULL`, titleID, a.MBID).Scan(&x.PartID, &x.Keep, &x.PartFingerprint); errors.Is(err, sql.ErrNoRows) {
			continue
		} else if err != nil {
			return s, err
		}
		if err := q.QueryRow("SELECT mbid, chosen_by FROM pressings WHERE part_id = ? AND chosen = 1 AND dropped_at IS NULL", x.PartID).Scan(&x.Chosen, &x.ChosenBy); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return s, err
		}
		x.Tracks = map[int64]bool{}
		x.TrackFingerprints = map[int64]string{}
		rows, err := q.Query("SELECT id, keep_up_to_date, created_at || char(31) || title_id || char(31) || part_id || char(31) || kind || char(31) || COALESCE(recording_id, '') || char(31) || numbers FROM items WHERE part_id = ?", x.PartID)
		if err != nil {
			return s, err
		}
		for rows.Next() {
			var id int64
			var keep bool
			var fp string
			if err := rows.Scan(&id, &keep, &fp); err != nil {
				rows.Close()
				return s, err
			}
			x.Tracks[id] = keep
			x.TrackFingerprints[id] = fp
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return s, err
		}
		rows.Close()
		s.Parts = append(s.Parts, x)
	}
	return s, nil
}

func restoreMusicState(tx *sql.Tx, titleID int64, s musicState, by domain.Actor, now string) error {
	who, _ := json.Marshal(by)
	// Validate every old selected pressing before changing any row. If one was
	// dropped after the move, undo must leave the current selection untouched.
	for _, p := range s.Parts {
		if p.PartFingerprint == "" || len(p.TrackFingerprints) != len(p.Tracks) {
			return errUndoChanged
		}
		if p.Chosen == "" {
			continue
		}
		var id int64
		if err := tx.QueryRow("SELECT id FROM pressings WHERE part_id=? AND mbid=? AND dropped_at IS NULL", p.PartID, p.Chosen).Scan(&id); err != nil {
			return fmt.Errorf("the prior pressing is no longer active: %w", err)
		}
	}
	res, err := tx.Exec("UPDATE titles SET keep_up_to_date=?, new_parts_policy=?, music_types=?, music_secondary=?, music_statuses=?, updated_at=?, updated_by=? WHERE id=?", s.KeepUpToDate, s.NewAlbums, s.Types, s.Secondary, s.Statuses, now, string(who), titleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errUndoChanged
	}
	for _, p := range s.Parts {
		res, err := tx.Exec("UPDATE parts SET keep_up_to_date=?, updated_at=?, updated_by=? WHERE id=? AND title_id=? AND created_at || char(31) || title_id || char(31) || kind || char(31) || source_ids = ?", p.Keep, now, string(who), p.PartID, titleID, p.PartFingerprint)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return errUndoChanged
		}
		for id, keep := range p.Tracks {
			res, err := tx.Exec("UPDATE items SET keep_up_to_date=?, updated_at=?, updated_by=? WHERE id=? AND part_id=? AND title_id=? AND created_at || char(31) || title_id || char(31) || part_id || char(31) || kind || char(31) || COALESCE(recording_id, '') || char(31) || numbers = ?", keep, now, string(who), id, p.PartID, titleID, p.TrackFingerprints[id])
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n != 1 {
				return errUndoChanged
			}
		}
		if _, err := tx.Exec("UPDATE pressings SET chosen=0, updated_at=?, updated_by=? WHERE part_id=?", now, string(who), p.PartID); err != nil {
			return err
		}
		if p.Chosen != "" {
			res, err := tx.Exec("UPDATE pressings SET chosen=1, chosen_by=?, updated_at=?, updated_by=? WHERE part_id=? AND mbid=? AND dropped_at IS NULL", p.ChosenBy, now, string(who), p.PartID, p.Chosen)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n != 1 {
				return errors.New("the prior pressing is no longer active")
			}
		}
	}
	return nil
}

func currentMusicState(ctx context.Context, tx *sql.Tx, titleID int64, moved musicState) (musicState, error) {
	var cur musicState
	if err := tx.QueryRowContext(ctx, "SELECT keep_up_to_date, new_parts_policy, music_types, music_secondary, music_statuses FROM titles WHERE id = ?", titleID).Scan(&cur.KeepUpToDate, &cur.NewAlbums, &cur.Types, &cur.Secondary, &cur.Statuses); errors.Is(err, sql.ErrNoRows) {
		return cur, errUndoGone
	} else if err != nil {
		return cur, err
	}
	for _, p := range moved.Parts {
		c := musicPartState{PartID: p.PartID, Tracks: map[int64]bool{}, TrackFingerprints: map[int64]string{}}
		if err := tx.QueryRowContext(ctx, "SELECT keep_up_to_date, created_at || char(31) || title_id || char(31) || kind || char(31) || source_ids FROM parts WHERE id=? AND title_id=?", p.PartID, titleID).Scan(&c.Keep, &c.PartFingerprint); errors.Is(err, sql.ErrNoRows) {
			return cur, errUndoChanged
		} else if err != nil {
			return cur, err
		}
		if err := tx.QueryRowContext(ctx, "SELECT mbid, chosen_by FROM pressings WHERE part_id=? AND chosen=1 AND dropped_at IS NULL", p.PartID).Scan(&c.Chosen, &c.ChosenBy); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return cur, err
		}
		rows, err := tx.QueryContext(ctx, "SELECT id, keep_up_to_date, created_at || char(31) || title_id || char(31) || part_id || char(31) || kind || char(31) || COALESCE(recording_id, '') || char(31) || numbers FROM items WHERE part_id=? AND title_id=?", p.PartID, titleID)
		if err != nil {
			return cur, err
		}
		for rows.Next() {
			var id int64
			var keep bool
			var fp string
			if err = rows.Scan(&id, &keep, &fp); err != nil {
				rows.Close()
				return cur, err
			}
			c.Tracks[id] = keep
			c.TrackFingerprints[id] = fp
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return cur, err
		}
		rows.Close()
		cur.Parts = append(cur.Parts, c)
	}
	return cur, nil
}

func undoMusicPolicyTx(ctx context.Context, tx *sql.Tx, x madeRow, old, moved musicState, by domain.Actor, now string) error {
	fp, err := readFingerprint(ctx, tx, "title", x.row)
	if errors.Is(err, sql.ErrNoRows) {
		return errUndoGone
	}
	if err != nil {
		return err
	}
	if fp != x.fp {
		return errUndoReplaced
	}
	cur, err := currentMusicState(ctx, tx, x.row, moved)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(cur, moved) {
		return errUndoChanged
	}
	return restoreMusicState(tx, x.row, old, by, now)
}

func undoAskSettingsTx(ctx context.Context, tx *sql.Tx, old, moved askSettingsMark, by domain.Actor, now string) error {
	for key, want := range moved {
		cur, err := readSettingValue(ctx, tx, key)
		if err != nil {
			return err
		}
		if cur != want {
			return errUndoChanged
		}
	}
	who, _ := json.Marshal(by)
	for key, state := range old {
		if !state.Exists {
			if _, err := tx.Exec("DELETE FROM settings WHERE grp = 'this_app' AND key = ?", key); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			key, state.Value, now, string(who), now, string(who)); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) applyMusicPolicyTx(tx *sql.Tx, titleID int64, p *MusicPolicyPlan) error {
	types, _ := json.Marshal(p.AlbumTypes)
	secondary, _ := json.Marshal(p.SecondaryTypes)
	statuses, _ := json.Marshal(p.ReleaseStatuses)
	who, _ := json.Marshal(r.by)
	now := database.FormatTime(r.m.now())
	var newAlbums any
	if p.NewAlbums != nil {
		newAlbums = *p.NewAlbums
	}
	if _, err := tx.Exec(`UPDATE titles SET new_parts_policy = COALESCE(?, new_parts_policy),
		music_types = CASE WHEN json_array_length(?) > 0 THEN ? ELSE music_types END,
		music_secondary = CASE WHEN json_array_length(?) > 0 THEN ? ELSE music_secondary END,
		music_statuses = CASE WHEN json_array_length(?) > 0 THEN ? ELSE music_statuses END,
		updated_at = ?, updated_by = ? WHERE id = ? AND kind = 'artist'`,
		newAlbums, string(types), string(types), string(secondary), string(secondary), string(statuses), string(statuses), now, string(who), titleID); err != nil {
		return err
	}
	for _, album := range p.Albums {
		var partID int64
		if err := tx.QueryRow(`SELECT id FROM parts WHERE title_id = ? AND kind = 'album' AND json_extract(source_ids, '$.musicbrainz') = ? AND dropped_at IS NULL`,
			titleID, album.MBID).Scan(&partID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("album %s is not in the adopted artist; its monitoring choice was not applied", album.MBID)
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE parts SET keep_up_to_date = ?, updated_at = ?, updated_by = ? WHERE id = ?", album.KeepUpToDate, now, string(who), partID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE items SET keep_up_to_date = ?, updated_at = ?, updated_by = ? WHERE part_id = ?", album.KeepUpToDate, now, string(who), partID); err != nil {
			return err
		}
		if album.PressingMBID == "" {
			continue
		}
		var pressingID int64
		if err := tx.QueryRow("SELECT id FROM pressings WHERE part_id = ? AND mbid = ? AND dropped_at IS NULL", partID, album.PressingMBID).Scan(&pressingID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("pressing %s is not in album %s; its watched choice was not applied", album.PressingMBID, album.MBID)
		} else if err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE pressings SET chosen = 0, updated_at = ?, updated_by = ? WHERE part_id = ? AND chosen = 1", now, string(who), partID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE pressings SET chosen = 1, chosen_by = 'person', updated_at = ?, updated_by = ? WHERE id = ?", now, string(who), pressingID); err != nil {
			return err
		}
	}
	return nil
}
