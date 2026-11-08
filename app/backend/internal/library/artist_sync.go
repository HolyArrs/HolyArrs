package library

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// syncAlbumsTx brings an artist's album rows in line with a. choice is the
// album choice when the artist is being added, "" on a refresh. Albums
// match by MusicBrainz id, so rows, Slots and Files survive. An album the
// source no longer lists is removed when nothing holds it, else flagged;
// when the source's list is cut short (a.Partial) nothing is removed.
func syncAlbumsTx(ctx context.Context, tx *sql.Tx, t mediafiles.Title, a metadata.Artist, choice string, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	parts, err := database.Select[mediafiles.Part](ctx, tx, "parts", "title_id = ? AND kind = 'album'", t.ID)
	if err != nil {
		return out, err
	}
	byMBID := map[string]*mediafiles.Part{}
	for i := range parts {
		byMBID[parts[i].SourceIDs["musicbrainz"]] = &parts[i]
	}
	var albums []metadata.Album
	seen := map[string]bool{}
	for _, al := range a.Albums {
		if al.MBID == "" || seen[al.MBID] {
			continue
		}
		seen[al.MBID] = true
		albums = append(albums, al)
	}
	var keepAtAdd map[string]bool
	if choice != "" {
		var ids []string
		dates := map[string]*time.Time{}
		for _, al := range albums {
			if AlbumKindsOK(t, al.PrimaryType, al.SecondaryTypes) {
				ids = append(ids, al.MBID)
				dates[al.MBID] = musicDate(al.ReleaseDate)
			}
		}
		keepAtAdd = albumChoiceKeep(choice, ids, dates, nil, now)
	}
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	for _, al := range albums {
		date, prec := musicDate(al.ReleaseDate), musicDatePrecision(al.ReleaseDate)
		typ := al.PrimaryType
		sec := al.SecondaryTypes
		if sec == nil {
			sec = []string{}
		}
		kinds := AlbumKindsOK(t, typ, sec)
		if p := byMBID[al.MBID]; p != nil {
			ok, err := statusOK(tx, t, p.ID)
			if err != nil {
				return out, err
			}
			included := (kinds || p.KeepUpToDate) && ok
			dateMoved := !EqTime(p.ReleaseDate, date)
			changed := p.Name != al.Name || !EqStr(p.ReleaseType, str(typ)) || !slices.Equal(p.SecondaryTypes, sec) || dateMoved ||
				!EqStr(p.ReleaseDatePrecision, prec) ||
				!EqStr(p.Disambiguation, str(al.Disambiguation)) || p.DroppedAt != nil || p.Included != included
			if !changed {
				continue
			}
			if included != p.Included {
				p.KeepUpToDate = included && newAlbumKeep(t, date)
				if _, err := tx.Exec("UPDATE items SET keep_up_to_date = ? WHERE part_id = ? AND kind = 'track'", p.KeepUpToDate, p.ID); err != nil {
					return out, err
				}
			}
			p.Name, p.ReleaseType, p.SecondaryTypes, p.ReleaseDate, p.Disambiguation, p.DroppedAt, p.Included = al.Name, str(typ), sec, date, str(al.Disambiguation), nil, included
			p.ReleaseDatePrecision = prec
			if err := database.Update(tx, "parts", p, by); err != nil {
				return out, err
			}
			if dateMoved {
				var d any
				if date != nil {
					d = database.FormatTime(*date)
				}
				if _, err := tx.Exec("UPDATE items SET air_date = ? WHERE part_id = ? AND kind = 'track'", d, p.ID); err != nil {
					return out, err
				}
			}
			if !included {
				res, err := dropTracksTx(tx, p.ID, nil, now, by)
				if err != nil {
					return out, err
				}
				out.add(res)
			}
			out.AlbumsChanged++
			continue
		}
		keep := kinds && t.KeepUpToDate
		if choice != "" {
			keep = keep && keepAtAdd[al.MBID]
		} else {
			keep = keep && newAlbumKeep(t, date)
		}
		p := &mediafiles.Part{TitleID: t.ID, Kind: "album", Name: al.Name, ReleaseType: str(typ), ReleaseDate: date, ReleaseDatePrecision: prec, KeepUpToDate: keep,
			SourceIDs: map[string]string{"musicbrainz": al.MBID}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: sec, Included: kinds,
			Disambiguation: str(al.Disambiguation)}
		if _, err := database.Insert(tx, "parts", p, by); err != nil {
			return out, err
		}
		out.AlbumsAdded++
	}
	for mbid, p := range byMBID {
		if seen[mbid] || a.Partial {
			continue
		}
		held, err := partHeld(tx, p.ID)
		if err != nil {
			return out, err
		}
		if !held {
			if _, err := tx.Exec("DELETE FROM parts WHERE id = ?", p.ID); err != nil {
				return out, err
			}
			out.AlbumsRemoved++
			continue
		}
		if p.DroppedAt == nil {
			d := now
			p.DroppedAt = &d
			if err := database.Update(tx, "parts", p, by); err != nil {
				return out, err
			}
			out.AlbumsDropped++
		}
	}
	return out, nil
}

// dropTracksTx removes an album's tracks that keep is nil for or that keep
// does not hold (item ids): deleted when nothing holds them, else kept and
// flagged.
func dropTracksTx(tx *sql.Tx, partID int64, keep map[int64]bool, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	rows, err := tx.Query("SELECT id, dropped_at IS NOT NULL FROM items WHERE part_id = ? AND kind = 'track'", partID)
	if err != nil {
		return out, err
	}
	type row struct {
		id      int64
		dropped bool
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.dropped); err != nil {
			rows.Close()
			return out, err
		}
		rs = append(rs, r)
	}
	rows.Close()
	for _, r := range rs {
		if keep[r.id] {
			continue
		}
		var held int
		if err := tx.QueryRow("SELECT count(*) FROM slots WHERE item_id = ? AND ("+HoldingSlots+")", r.id).Scan(&held); err != nil {
			return out, err
		}
		if held == 0 {
			if _, err := tx.Exec("DELETE FROM items WHERE id = ?", r.id); err != nil {
				return out, err
			}
			out.TracksRemoved++
			continue
		}
		if !r.dropped {
			if _, err := tx.Exec("UPDATE items SET dropped_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				database.FormatTime(now), database.FormatTime(now), ActorText(by), r.id); err != nil {
				return out, err
			}
			out.TracksDropped++
		}
	}
	return out, nil
}

// syncPressingsTx brings an album's Pressings in line with ps, keeps the
// chosen Pressing (a person's choice, or the rule's earlier pick) while the
// source lists it, else chooses by rule, and makes the album's tracks the
// chosen Pressing's tracks. An album none of whose Pressings has a counted
// status stops being included. An empty ps changes nothing; with cut (the
// source's list was cut short) Pressings it did not list are kept.
func syncPressingsTx(ctx context.Context, tx *sql.Tx, t mediafiles.Title, partID int64, ps []metadata.Pressing, cut bool, rule string, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	if len(ps) == 0 {
		return out, nil
	}
	var p mediafiles.Part
	if err := database.Get(ctx, tx, "parts", partID, &p); err != nil {
		return out, err
	}
	rows, err := database.Select[domain.AlbumPressing](ctx, tx, "pressings", "part_id = ?", partID)
	if err != nil {
		return out, err
	}
	have := map[string]*domain.AlbumPressing{}
	for i := range rows {
		have[rows[i].MBID] = &rows[i]
	}
	listed := map[string]bool{}
	var kept []domain.AlbumPressing
	for _, mp := range ps {
		if listed[mp.MBID] {
			continue
		}
		listed[mp.MBID] = true
		tracks := make([]domain.PressingTrack, 0, len(mp.Tracks))
		for _, tr := range mp.Tracks {
			tracks = append(tracks, domain.PressingTrack(tr))
		}
		formats := mp.Formats
		if formats == nil {
			formats = []string{}
		}
		row := have[mp.MBID]
		if row == nil {
			row = &domain.AlbumPressing{PartID: partID, MBID: mp.MBID, ChosenBy: "auto"}
		}
		row.Name, row.Status, row.Country, row.Date, row.Disambiguation, row.Formats, row.TrackCount, row.Tracks, row.DroppedAt =
			mp.Name, mp.Status, mp.Country, mp.Date, mp.Disambiguation, formats, len(tracks), tracks, nil
		if row.ID == 0 {
			if _, err := database.Insert(tx, "pressings", row, by); err != nil {
				return out, err
			}
		} else if err := database.Update(tx, "pressings", row, by); err != nil {
			return out, err
		}
		kept = append(kept, *row)
	}
	for mbid, row := range have {
		if !listed[mbid] && cut {
			kept = append(kept, *row)
		} else if !listed[mbid] {
			if _, err := tx.Exec("DELETE FROM pressings WHERE id = ?", row.ID); err != nil {
				return out, err
			}
		}
	}
	countable := false
	for _, k := range kept {
		countable = countable || slices.Contains(t.MusicStatuses, k.Status)
	}
	if len(kept) > 0 && !countable {
		if p.Included {
			p.Included, p.KeepUpToDate = false, false
			if err := database.Update(tx, "parts", &p, by); err != nil {
				return out, err
			}
			out.AlbumsChanged++
		}
		res, err := dropTracksTx(tx, partID, nil, now, by)
		out.add(res)
		return out, err
	}
	chosen := -1
	for i, k := range kept {
		if k.Chosen {
			chosen = i
		}
	}
	if chosen < 0 {
		chosen = ChoosePressing(kept, t.MusicStatuses, rule)
		if chosen >= 0 {
			if _, err := tx.Exec("UPDATE pressings SET chosen = 0 WHERE part_id = ?", partID); err != nil {
				return out, err
			}
			if _, err := tx.Exec("UPDATE pressings SET chosen = 1, chosen_by = 'auto' WHERE id = ?", kept[chosen].ID); err != nil {
				return out, err
			}
			out.PressingsChosen++
		}
	}
	var tracks []domain.PressingTrack
	if chosen >= 0 {
		tracks = kept[chosen].Tracks
	}
	res, err := syncTracksTx(ctx, tx, t.ID, p, tracks, now, by)
	out.add(res)
	return out, err
}

// syncTracksTx makes an album's track rows the tracks given (the chosen
// Pressing's). Rows match by track id, then recording id, then disc and
// number, so their Slots and Files stay; tracks no longer there are
// removed, or kept and flagged when a File or Download holds them; new
// tracks get one Slot per Version, not out yet or wanted by the album's
// release date. No tracks changes nothing: an empty list is never taken
// to mean every track went.
func syncTracksTx(ctx context.Context, tx *sql.Tx, titleID int64, p mediafiles.Part, tracks []domain.PressingTrack, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	if len(tracks) == 0 {
		return out, nil
	}
	items, err := database.Select[mediafiles.Item](ctx, tx, "items", "part_id = ? AND kind = 'track' ORDER BY id", p.ID)
	if err != nil {
		return out, err
	}
	versions, err := database.Select[mediafiles.Version](ctx, tx, "versions", "title_id = ?", titleID)
	if err != nil {
		return out, err
	}
	claimed := map[int64]bool{}
	match := make([]*mediafiles.Item, len(tracks))
	find := func(ok func(it mediafiles.Item, tr domain.PressingTrack) bool) {
		for i, tr := range tracks {
			if match[i] != nil {
				continue
			}
			for j := range items {
				if !claimed[items[j].ID] && ok(items[j], tr) {
					match[i], claimed[items[j].ID] = &items[j], true
					break
				}
			}
		}
	}
	find(func(it mediafiles.Item, tr domain.PressingTrack) bool {
		return tr.ID != "" && it.SourceIDs["musicbrainz"] == tr.ID
	})
	find(func(it mediafiles.Item, tr domain.PressingTrack) bool {
		return tr.RecordingID != "" && it.RecordingID != nil && *it.RecordingID == tr.RecordingID
	})
	find(func(it mediafiles.Item, tr domain.PressingTrack) bool {
		return it.Numbers.Disc != nil && it.Numbers.Track != nil && *it.Numbers.Disc == tr.Disc && *it.Numbers.Track == tr.Number
	})
	res, err := dropTracksTx(tx, p.ID, claimed, now, by)
	if err != nil {
		return out, err
	}
	out.add(res)
	open, reason := AlbumGate(p.ReleaseDate, now)
	for i, tr := range tracks {
		name, rec := tr.Name, tr.RecordingID
		var length *int
		if tr.LengthMS > 0 {
			l := tr.LengthMS
			length = &l
		}
		disc, num := tr.Disc, tr.Number
		if it := match[i]; it != nil {
			changed := !EqStr(it.Name, &name) || !EqStr(it.RecordingID, &rec) || !EqInt(it.LengthMS, length) || it.SourceIDs["musicbrainz"] != tr.ID ||
				!EqInt(it.Numbers.Disc, &disc) || !EqInt(it.Numbers.Track, &num) || it.DroppedAt != nil || !EqTime(it.AirDate, p.ReleaseDate)
			if !changed {
				continue
			}
			ids := map[string]string{}
			for k, v := range it.SourceIDs {
				ids[k] = v
			}
			ids["musicbrainz"] = tr.ID
			it.Name, it.RecordingID, it.LengthMS, it.SourceIDs, it.Numbers.Disc, it.Numbers.Track, it.DroppedAt, it.AirDate =
				&name, &rec, length, ids, &disc, &num, nil, p.ReleaseDate
			if err := database.Update(tx, "items", it, by); err != nil {
				return out, err
			}
			out.TracksChanged++
			continue
		}
		it := &mediafiles.Item{TitleID: titleID, PartID: &p.ID, Kind: "track", Numbers: mediafiles.Numbers{Disc: &disc, Track: &num}, Name: &name,
			AirDate: p.ReleaseDate, KeepUpToDate: p.KeepUpToDate, RecordingID: &rec, LengthMS: length, SourceIDs: map[string]string{"musicbrainz": tr.ID}}
		if _, err := database.Insert(tx, "items", it, by); err != nil {
			return out, err
		}
		out.TracksAdded++
		state := mediafiles.SlotNotOutYet
		if open {
			state = mediafiles.SlotWanted
		}
		for _, v := range versions {
			r := reason
			if _, err := database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state, LastReason: &r}, by); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// ErrNotMusicRules says Pick rules for movies and shows were given for an
// artist: they have no music steps, so they could never pick an album.
var ErrNotMusicRules = errors.New("these Pick rules are for movies and shows; choose music Pick rules (Lossless first, Lossy 320 or V0, Any music)")

// musicRulesTx says the Pick rules judge music.
func musicRulesTx(tx *sql.Tx, id int64) bool {
	var pr mediafiles.PickRules
	if err := database.Get(context.Background(), tx, "pick_rules", id, &pr); err != nil {
		return false
	}
	return mediafiles.IsMusicRules(pr.Body)
}
