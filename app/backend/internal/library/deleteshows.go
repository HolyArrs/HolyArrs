package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// Refusals of a season or episode delete.
var (
	ErrNeverAddWholeShow = errors.New("Never-add is for a whole show; delete the show to add it to Never-add")
	ErrNoEpisodesChosen  = errors.New("choose a season or at least one episode of this show")
	ErrNoSuchEpisode     = errors.New("an episode chosen is not one of this show's")
	ErrNeverAddWholeArt  = errors.New("Never-add is for a whole artist; delete the artist to add it to Never-add")
	ErrNoSuchAlbumHere   = errors.New("the album chosen is not one of this artist's")
	ErrNoSuchTrack       = errors.New("a track chosen is not one of this artist's")
	ErrMixedScope        = errors.New("choose a season or episodes of a show, or an album or tracks of an artist, one at a time")
)

// RemoveScope is the part of a show or artist a delete takes: one season or
// some episodes (episode ids) of a show, one album (part id) or some tracks
// (track ids) of an artist. Empty means the whole title.
type RemoveScope struct {
	Season     *int    `json:"season,omitempty"`
	EpisodeIDs []int64 `json:"episode_ids,omitempty"`
	AlbumID    *int64  `json:"album_id,omitempty"`
	TrackIDs   []int64 `json:"track_ids,omitempty"`
}

// Whole says the scope is the whole title.
func (sc RemoveScope) Whole() bool {
	return sc.Season == nil && len(sc.EpisodeIDs) == 0 && sc.AlbumID == nil && len(sc.TrackIDs) == 0
}

// music says the scope is part of an artist.
func (sc RemoveScope) music() bool { return sc.AlbumID != nil || len(sc.TrackIDs) > 0 }

// Check says what is wrong with a scope: more than one part chosen.
func (sc RemoveScope) Check() error {
	n := 0
	for _, set := range []bool{sc.Season != nil, len(sc.EpisodeIDs) > 0, sc.AlbumID != nil, len(sc.TrackIDs) > 0} {
		if set {
			n++
		}
	}
	if n > 1 {
		return ErrMixedScope
	}
	return nil
}

// planRemoveEpisodes works out deleting a season or some episodes of a
// show. Nothing changes. A file is held only when every episode it holds
// is deleted; a multi-episode file that also holds an episode that stays
// is kept for it.
func planRemoveEpisodes(ctx context.Context, db Reader, titleID int64, sc RemoveScope, c RemoveChoice, now time.Time) (RemovePlan, mediafiles.Title, map[int64]bool, error) {
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", titleID, &t); err != nil {
		return RemovePlan{}, t, nil, err
	}
	if err := sc.Check(); err != nil {
		return RemovePlan{}, t, nil, err
	}
	switch {
	case sc.music() && t.Kind != "artist":
		return RemovePlan{}, t, nil, ErrNotArtist
	case !sc.music() && t.Kind != "show":
		return RemovePlan{}, t, nil, ErrNotShow
	}
	if t.State == "removed" {
		return RemovePlan{}, t, nil, ErrAlreadyRemoved
	}
	if c.NeverAdd && sc.music() {
		return RemovePlan{}, t, nil, ErrNeverAddWholeArt
	}
	if c.NeverAdd {
		return RemovePlan{}, t, nil, ErrNeverAddWholeShow
	}
	var items []mediafiles.Item
	var err error
	albumName := ""
	switch {
	case sc.AlbumID != nil:
		if err := db.QueryRowContext(ctx, "SELECT name FROM parts WHERE id = ? AND title_id = ? AND kind = 'album'", *sc.AlbumID, t.ID).Scan(&albumName); err != nil {
			return RemovePlan{}, t, nil, ErrNoSuchAlbumHere
		}
		items, err = database.Select[mediafiles.Item](ctx, db, "items", "title_id = ? AND kind = 'track' AND part_id = ? ORDER BY id", t.ID, *sc.AlbumID)
	case len(sc.TrackIDs) > 0:
		args := []any{t.ID}
		for _, id := range sc.TrackIDs {
			args = append(args, id)
		}
		items, err = database.Select[mediafiles.Item](ctx, db, "items", "title_id = ? AND kind = 'track' AND id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(sc.TrackIDs)), ",")+") ORDER BY id", args...)
		if err == nil && len(items) != len(uniq(sc.TrackIDs)) {
			return RemovePlan{}, t, nil, ErrNoSuchTrack
		}
	case sc.Season != nil:
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", t.ID, *sc.Season).Scan(&n); err != nil {
			return RemovePlan{}, t, nil, err
		}
		if n == 0 {
			return RemovePlan{}, t, nil, ErrNoSuchSeason
		}
		items, err = database.Select[mediafiles.Item](ctx, db, "items", "title_id = ? AND kind = 'episode' AND json_extract(numbers, '$.season') = ? ORDER BY id", t.ID, *sc.Season)
	case len(sc.EpisodeIDs) > 0:
		args := []any{t.ID}
		for _, id := range sc.EpisodeIDs {
			args = append(args, id)
		}
		items, err = database.Select[mediafiles.Item](ctx, db, "items", "title_id = ? AND kind = 'episode' AND id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(sc.EpisodeIDs)), ",")+") ORDER BY id", args...)
		if err == nil && len(items) != len(uniq(sc.EpisodeIDs)) {
			return RemovePlan{}, t, nil, ErrNoSuchEpisode
		}
	default:
		return RemovePlan{}, t, nil, ErrNoEpisodesChosen
	}
	if err != nil {
		return RemovePlan{}, t, nil, err
	}
	rules, err := importer.ReadFileRules(ctx, db)
	if err != nil {
		return RemovePlan{}, t, nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i].Numbers, items[j].Numbers
		if Deref(a.Season) != Deref(b.Season) {
			return Deref(a.Season) < Deref(b.Season)
		}
		if Deref(a.Disc) != Deref(b.Disc) {
			return Deref(a.Disc) < Deref(b.Disc)
		}
		if Deref(a.Track) != Deref(b.Track) {
			return Deref(a.Track) < Deref(b.Track)
		}
		return Deref(a.Episode) < Deref(b.Episode)
	})
	scope := "episodes"
	switch {
	case sc.Season != nil:
		scope = "season"
	case sc.AlbumID != nil:
		scope = "album"
	case len(sc.TrackIDs) > 0:
		scope = "tracks"
	}
	p := RemovePlan{TitleID: t.ID, Name: t.Name, Scope: scope, Season: sc.Season, Episodes: []string{}, Choice: c, Held: []RemoveFile{}, Kept: []RemoveFile{},
		HoldingDays: rules.HoldingDays, AlbumID: sc.AlbumID, Album: albumName}
	label := func(sid int64) string { return EpisodeLabel(ctx, db, sid) }
	if sc.music() {
		p.Tracks = []string{}
		label = func(sid int64) string { return trackLabelOf(ctx, db, sid) }
	}
	set := map[int64]bool{}
	p.itemIDs = []int64{}
	for _, it := range items {
		p.itemIDs = append(p.itemIDs, it.ID)
		set[it.ID] = true
		if sc.music() {
			p.Tracks = append(p.Tracks, trackItemLabel(ctx, db, it))
			continue
		}
		p.Episodes = append(p.Episodes, fmt.Sprintf("S%02dE%02d", Deref(it.Numbers.Season), Deref(it.Numbers.Episode)))
	}
	files, err := database.Select[mediafiles.File](ctx, db, "files",
		"state IN ('present', 'missing') AND version_id IN (SELECT id FROM versions WHERE title_id = ?) ORDER BY id", t.ID)
	if err != nil {
		return p, t, nil, err
	}
	for _, f := range files {
		var in, out []string
		for _, sid := range f.SlotIDs {
			var itemID int64
			if db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", sid).Scan(&itemID) != nil {
				continue
			}
			if set[itemID] {
				in = append(in, label(sid))
			} else {
				out = append(out, label(sid))
			}
		}
		if len(in) == 0 {
			continue // not a file of these episodes
		}
		var v mediafiles.Version
		var lf mediafiles.LibraryFolder
		if err := database.Get(ctx, db, "versions", f.VersionID, &v); err != nil {
			return p, t, nil, err
		}
		if err := database.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
			return p, t, nil, err
		}
		rf := RemoveFile{FileID: f.ID, LibraryFolderID: lf.ID, Origin: f.Origin, Path: f.RelativePath, Episodes: append(in, out...), Protected: f.Protected, root: lf.Path}
		if f.Protected {
			p.Protected = append(p.Protected, rf)
			if !c.AcknowledgeProtection || !c.DeleteFiles || len(out) > 0 || f.Origin != "imported" && f.Origin != "adopted" {
				p.Skipped = true
			}
		}
		switch {
		case !c.DeleteFiles:
			rf.Why = "kept: files stay on disk unless you choose to delete them"
		case f.Origin != "imported" && f.Origin != "adopted":
			rf.Why = "kept: found on disk, not imported or adopted; HolyArrs never deletes it"
		case len(out) > 0:
			rf.Why = "kept: it also holds " + strings.Join(out, ", ") + ", which stays"
		case f.State == "missing":
			rf.Why = "kept: the file is not on disk"
		default:
			to, err := importer.HoldPath(lf.Path, f.RelativePath, now)
			if err != nil {
				rf.Why = "kept: " + err.Error()
				break
			}
			rel, _ := filepath.Rel(lf.Path, to)
			rf.To = filepath.ToSlash(rel)
			rf.Why = fmt.Sprintf("goes to the holding folder for %d days", rules.HoldingDays)
			if rules.HoldingDays == 0 {
				rf.Why = "goes to the holding folder until you empty it"
			}
			rf.Subtitles = holdSubtitles(lf.Path, f.RelativePath, now)
			p.Held = append(p.Held, rf)
			continue
		}
		p.Kept = append(p.Kept, rf)
	}
	if p.Skipped {
		for _, h := range p.Held {
			h.To = ""
			h.Why = ErrProtectedCopy.Error()
			p.Kept = append(p.Kept, h)
		}
		p.Held = []RemoveFile{}
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM downloads WHERE state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')
		AND EXISTS (SELECT 1 FROM json_each(downloads.slot_ids) j JOIN slots s ON s.id = j.value WHERE s.item_id IN (SELECT id FROM items WHERE title_id = ? AND `+
		itemFilter(sc)+`))`, append([]any{t.ID}, itemArgs(sc)...)...).Scan(&p.ActiveDownloads)
	return p, t, set, nil
}

func itemFilter(sc RemoveScope) string {
	switch {
	case sc.Season != nil:
		return "json_extract(numbers, '$.season') = ?"
	case sc.AlbumID != nil:
		return "part_id = ?"
	case len(sc.TrackIDs) > 0:
		return "id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(sc.TrackIDs)), ",") + ")"
	}
	return "id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(sc.EpisodeIDs)), ",") + ")"
}

func itemArgs(sc RemoveScope) []any {
	switch {
	case sc.Season != nil:
		return []any{*sc.Season}
	case sc.AlbumID != nil:
		return []any{*sc.AlbumID}
	}
	ids := sc.EpisodeIDs
	if len(sc.TrackIDs) > 0 {
		ids = sc.TrackIDs
	}
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// trackLabelOf names the track a Slot is for: "Old Days: track 03 Song",
// "Old Days: disc 2 track 01 Song" on an album of more than one disc.
func trackLabelOf(ctx context.Context, db Reader, slotID int64) string {
	var itemID int64
	if db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", slotID).Scan(&itemID) != nil {
		return ""
	}
	var it mediafiles.Item
	if database.Get(ctx, db, "items", itemID, &it) != nil {
		return ""
	}
	return trackItemLabel(ctx, db, it)
}

// trackItemLabel names a track item with its album.
func trackItemLabel(ctx context.Context, db Reader, it mediafiles.Item) string {
	album := ""
	discs := 1
	if it.PartID != nil {
		_ = db.QueryRowContext(ctx, "SELECT name FROM parts WHERE id = ?", *it.PartID).Scan(&album)
		var d sql.NullInt64
		_ = db.QueryRowContext(ctx, "SELECT max(json_extract(numbers, '$.disc')) FROM items WHERE part_id = ? AND kind = 'track' AND dropped_at IS NULL", *it.PartID).Scan(&d)
		discs = max(1, int(d.Int64))
	}
	l := fmt.Sprintf("track %02d", Deref(it.Numbers.Track))
	if discs > 1 {
		l = fmt.Sprintf("disc %d track %02d", max(1, Deref(it.Numbers.Disc)), Deref(it.Numbers.Track))
	}
	if it.Name != nil && *it.Name != "" {
		l += " " + *it.Name
	}
	if album != "" {
		l = album + ": " + l
	}
	return l
}

func uniq(ids []int64) map[int64]bool {
	m := map[int64]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// RemoveEpisodes deletes a season or some episodes of a show from the
// Library (apply false: only the plan). The episodes stop being kept up
// to date (the season too, for a season), and with delete_files their
// imported and adopted files go to the holding folder, so their Slots look
// for nothing until Keep up to date is turned on again. The show stays.
func (s *Service) RemoveEpisodes(ctx context.Context, titleID int64, sc RemoveScope, c RemoveChoice, apply bool, by domain.Actor) (RemovePlan, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	p, t, set, err := planRemoveEpisodes(ctx, s.Store.Read(), titleID, sc, c, s.now())
	if err != nil || !apply {
		return p, err
	}
	if p.Skipped {
		return p, ErrProtectedCopy
	}
	if err := s.holdFiles(ctx, &p, t.ID, by); err != nil {
		return p, err
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		ts, who := database.FormatTime(s.now()), ActorText(by)
		for id := range set {
			if _, err := tx.Exec("UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE id = ?", ts, who, id); err != nil {
				return err
			}
		}
		if sc.Season != nil {
			if _, err := tx.Exec("UPDATE parts SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE title_id = ? AND kind = 'season' AND number = ?",
				ts, who, t.ID, *sc.Season); err != nil {
				return err
			}
		}
		if sc.AlbumID != nil {
			if _, err := tx.Exec("UPDATE parts SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE id = ? AND title_id = ? AND kind = 'album'",
				ts, who, *sc.AlbumID, t.ID); err != nil {
				return err
			}
		}
		// The episodes of a held file have no copy any more.
		for _, h := range p.Held {
			if _, err := tx.Exec(`UPDATE slots SET file_id = NULL, good_enough = 0, state = 'wanted', last_reason = 'its file was deleted',
				updated_at = ?, updated_by = ? WHERE file_id = ? AND state IN ('in_library', 'better_wanted')`, ts, who, h.FileID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return p, err
	}
	if c.DeleteFiles {
		s.removeEmptyFolders(ctx, t.ID, p.Held, false)
	}
	p.Applied = true
	what := t.Name + " " + strings.Join(p.Episodes, ", ")
	switch {
	case sc.Season != nil:
		what = fmt.Sprintf("%s season %d", t.Name, *sc.Season)
	case sc.AlbumID != nil:
		what = t.Name + " – " + p.Album
	case sc.music() && len(p.Tracks) > 3:
		what = fmt.Sprintf("%s: %d tracks", t.Name, len(p.Tracks))
	case sc.music():
		what = t.Name + " " + strings.Join(p.Tracks, ", ")
	}
	data := map[string]any{"season": sc.Season, "episodes": p.Episodes, "item_ids": p.itemIDs, "delete_files": c.DeleteFiles, "held": len(p.Held), "kept": len(p.Kept)}
	if sc.music() {
		// The album folders the files were in, for media servers.
		folders := []string{}
		seen := map[string]bool{}
		for _, f := range append(append([]RemoveFile{}, p.Held...), p.Kept...) {
			if d := path.Dir(f.Path); d != "." && !seen[d] {
				seen[d] = true
				folders = append(folders, d+"/")
			}
		}
		data = map[string]any{"album_id": sc.AlbumID, "album": p.Album, "tracks": p.Tracks, "item_ids": p.itemIDs, "delete_files": c.DeleteFiles,
			"held": len(p.Held), "kept": len(p.Kept), "folders": folders}
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventEpisodesRemoved, TitleID: &t.ID, Summary: "Deleted " + what + " from the Library",
		Data: data, GroupKey: GroupKey(t.ID)})
	s.Publish("title", map[string]any{"id": t.ID})
	return p, nil
}
