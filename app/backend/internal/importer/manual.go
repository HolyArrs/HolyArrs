package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ByHandMax caps how many files one by-hand scan returns.
const ByHandMax = 2000

// ByHandGuess is the match the server guesses for a by-hand file.
type ByHandGuess struct {
	TitleID    int64   `json:"title_id,omitempty"`
	TitleName  string  `json:"title_name,omitempty"`
	TitleKind  string  `json:"title_kind,omitempty"`
	Year       int     `json:"year,omitempty"`
	VersionID  int64   `json:"version_id,omitempty"`
	Season     *int    `json:"season,omitempty"`
	Episode    *int    `json:"episode,omitempty"`
	SlotID     int64   `json:"slot_id,omitempty"`
	HasFile    bool    `json:"has_file,omitempty"`
	Confidence float64 `json:"confidence"`
	Detail     string  `json:"detail,omitempty"`
}

// ByHandFile is one file found by a by-hand scan.
type ByHandFile struct {
	Path      string      `json:"path"`
	Name      string      `json:"name"`
	Size      int64       `json:"size"`
	Kind      string      `json:"kind"`
	Quality   string      `json:"quality,omitempty"`
	Guess     ByHandGuess `json:"guess"`
	Note      string      `json:"note,omitempty"`
	InLibrary bool        `json:"in_library,omitempty"`
}

// ByHandScan is the result of scanning a path by hand.
type ByHandScan struct {
	Path  string       `json:"path"`
	Files []ByHandFile `json:"files"`
	More  bool         `json:"more"`
}

// ByHandItem is one file the user chose to import by hand.
type ByHandItem struct {
	Path      string `json:"path"`
	SlotID    int64  `json:"slot_id"`
	TitleID   int64  `json:"title_id"`
	VersionID int64  `json:"version_id"`
	Season    *int   `json:"season"`
	Episode   *int   `json:"episode"`
	Mode      string `json:"mode"`
}

// ByHandResult is the outcome of importing one by-hand item.
type ByHandResult struct {
	Path     string `json:"path"`
	Outcome  string `json:"outcome"`
	PlacedBy string `json:"placed_by,omitempty"`
	Target   string `json:"target,omitempty"`
	TitleID  int64  `json:"title_id,omitempty"`
	SlotID   int64  `json:"slot_id,omitempty"`
	FileID   int64  `json:"file_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ErrByHandPath is returned when a by-hand path is not usable.
var ErrByHandPath = errors.New("give the full path of a folder or file the server can read")

var byHandStat = os.Stat

// CleanByHandPath validates a by-hand path against the allowed places.
func CleanByHandPath(p string, places ByHandPlaces) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsRune(p, 0) || !filepath.IsAbs(p) {
		return "", ErrByHandPath
	}
	if strings.HasPrefix(filepath.VolumeName(p), `\\`) {
		return "", ErrByHandPath
	}
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part == ".." {
			return "", ErrByHandPath
		}
	}
	p = filepath.Clean(p)
	if places.within(p) == "" {
		return "", places.Refusal()
	}
	if _, err := byHandStat(p); err != nil {
		return "", ErrByHandPath
	}
	if !places.Holds(p) {
		return "", places.Refusal()
	}
	return p, nil
}

func libraryFile(ctx context.Context, db *sql.DB, folders []mediafiles.LibraryFolder, p string) bool {
	for _, lf := range folders {
		if !under(lf.Path, p) {
			continue
		}
		rel, err := filepath.Rel(lf.Path, p)
		if err != nil {
			continue
		}
		var n int
		// A found note whose episodes are all still empty does not count: the import links it.
		if db.QueryRowContext(ctx, `SELECT count(*) FROM files f JOIN versions v ON v.id = f.version_id
			WHERE v.library_folder_id = ? AND f.relative_path = ? AND f.state = 'present' AND f.slot_ids != '[]'
			AND NOT (f.origin = 'found' AND NOT EXISTS (SELECT 1 FROM json_each(f.slot_ids) j JOIN slots s ON s.id = j.value WHERE s.file_id IS NOT NULL))`,
			lf.ID, filepath.ToSlash(rel)).Scan(&n) == nil && n > 0 {
			return true
		}
	}
	return false
}

type byHandTitle struct {
	t     mediafiles.Title
	main  int64
	slots map[int64]mediafiles.Slot
	refs  []mediafiles.EpisodeRef
	movie int64
}

func (im *Importer) byHandTitle(ctx context.Context, db *sql.DB, cache map[int64]*byHandTitle, id int64) (*byHandTitle, error) {
	if bt := cache[id]; bt != nil {
		return bt, nil
	}
	bt := &byHandTitle{slots: map[int64]mediafiles.Slot{}}
	if err := database.Get(ctx, db, "titles", id, &bt.t); err != nil {
		return nil, err
	}
	_ = db.QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1", id).Scan(&bt.main)
	items, err := database.Select[mediafiles.Item](ctx, db, "items", "title_id = ? AND kind IN ('movie', 'episode') AND dropped_at IS NULL", id)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Kind == "movie" {
			bt.movie = it.ID
			continue
		}
		bt.refs = append(bt.refs, episodeRef(it))
	}
	slots, err := database.Select[mediafiles.Slot](ctx, db, "slots", "version_id = ?", bt.main)
	if err != nil {
		return nil, err
	}
	for _, s := range slots {
		bt.slots[s.ItemID] = s
	}
	cache[id] = bt
	return bt, nil
}

func episodeRef(it mediafiles.Item) mediafiles.EpisodeRef {
	r := mediafiles.EpisodeRef{ItemID: it.ID}
	if it.Numbers.Season != nil {
		r.Season = *it.Numbers.Season
	}
	if it.Numbers.Episode != nil {
		r.Episode = *it.Numbers.Episode
	}
	if it.Numbers.Absolute != nil {
		r.Absolute = *it.Numbers.Absolute
	}
	if it.Numbers.AltSeason != nil && it.Numbers.AltEpisode != nil {
		r.AltSeason, r.AltEpisode = *it.Numbers.AltSeason, *it.Numbers.AltEpisode
	}
	if it.AirDay != nil {
		r.AirDay = *it.AirDay
	}
	return r
}

// ScanByHand lists the importable files under root.
func (im *Importer) ScanByHand(ctx context.Context, root string) (ByHandScan, error) {
	out := ByHandScan{Path: root, Files: []ByHandFile{}}
	db := im.Store.Read()
	folders, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "1 = 1")
	if err != nil {
		return out, err
	}
	places, err := im.byHandPlaces(ctx, db, folders)
	if err != nil {
		return out, err
	}
	if root, err = CleanByHandPath(root, places); err != nil {
		return out, err
	}
	out.Path = root
	titles, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind IN ('movie', 'show') AND state != 'removed'")
	if err != nil {
		return out, err
	}
	var refs []mediafiles.TitleRef
	for _, t := range titles {
		year := 0
		if t.Year != nil {
			year = *t.Year
		}
		refs = append(refs, mediafiles.TitleRef{ID: t.ID, Name: t.Name, Aliases: t.Aliases, Year: year, Kind: t.Kind})
	}
	cache := map[int64]*byHandTitle{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		kind := ""
		switch {
		case videoExt[ext]:
			kind = "video"
		case audioExt[ext]:
			kind = "audio"
		default:
			return nil
		}
		if len(out.Files) == ByHandMax {
			out.More = true
			return filepath.SkipAll
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		f := ByHandFile{Path: p, Name: filepath.Base(p), Size: fi.Size(), Kind: kind}
		if rel, err := filepath.Rel(root, p); err == nil && rel != "." {
			f.Name = filepath.ToSlash(rel)
		}
		f.InLibrary = libraryFile(ctx, db, folders, p)
		if kind == "audio" {
			f.Note = "music is not imported by hand yet"
			out.Files = append(out.Files, f)
			return nil
		}
		if err := im.guessByHand(ctx, db, cache, refs, &f); err != nil {
			return err
		}
		out.Files = append(out.Files, f)
		return nil
	})
	return out, err
}

func (im *Importer) guessByHand(ctx context.Context, db *sql.DB, cache map[int64]*byHandTitle, refs []mediafiles.TitleRef, f *ByHandFile) error {
	p := releasename.Parse(filepath.Base(f.Path), releasename.Hints{Size: f.Size})
	if p.Title.Value == "" {
		p = releasename.Parse(filepath.Base(filepath.Dir(f.Path)), releasename.Hints{Size: f.Size})
	}
	f.Quality = mediafiles.StepFor(p)
	if isSample(f.Path, p) {
		f.Note = "a sample, not the full video"
	}
	if f.InLibrary {
		f.Note = "already a file in the Library"
	}
	m := mediafiles.MatchTitle(p, refs)
	if m.Title == nil {
		if fp := releasename.Parse(filepath.Base(filepath.Dir(f.Path)), releasename.Hints{Size: f.Size}); fp.Title.Value != "" {
			if fm := mediafiles.MatchTitle(fp, refs); fm.Title != nil {
				m = fm
			}
		}
	}
	f.Guess.Detail = m.Detail
	if m.Title == nil {
		return nil
	}
	bt, err := im.byHandTitle(ctx, db, cache, m.Title.ID)
	if err != nil {
		return err
	}
	f.Guess.TitleID, f.Guess.TitleName, f.Guess.TitleKind, f.Guess.Year = bt.t.ID, bt.t.Name, bt.t.Kind, m.Title.Year
	f.Guess.VersionID, f.Guess.Confidence = bt.main, confidence(m.Conf)
	item := bt.movie
	if bt.t.Kind == "show" {
		item = 0
		order := mediafiles.OrderAired
		if bt.t.EpisodeOrder != nil && *bt.t.EpisodeOrder != "" {
			order = *bt.t.EpisodeOrder
		}
		sp := releasename.Parse(filepath.Base(f.Path), releasename.Hints{Size: f.Size, Show: true})
		found, why := mediafiles.MatchEpisodes(sp, order, bt.refs)
		if len(found) == 0 {
			f.Guess.Detail = why
			return nil
		}
		season, episode := found[0].Season, found[0].Episode
		f.Guess.Season, f.Guess.Episode, item = &season, &episode, found[0].ItemID
	}
	if s, ok := bt.slots[item]; ok && item != 0 {
		f.Guess.SlotID, f.Guess.HasFile = s.ID, s.FileID != nil
	}
	return nil
}

// ImportByHand imports the chosen items and returns one result each.
func (im *Importer) ImportByHand(ctx context.Context, items []ByHandItem, by domain.Actor) ([]ByHandResult, error) {
	unlock := im.Store.LockFiles()
	defer unlock()
	db := im.Store.Read()
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	folders, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "1 = 1")
	if err != nil {
		return nil, err
	}
	places, err := im.byHandPlaces(ctx, db, folders)
	if err != nil {
		return nil, err
	}
	out := make([]ByHandResult, 0, len(items))
	for _, it := range items {
		r, err := im.importOneByHand(ctx, db, ns, folders, places, it, by)
		if err != nil {
			r.Outcome, r.Error = "failed", err.Error()
		}
		out = append(out, r)
	}
	return out, nil
}

func byHandSlot(ctx context.Context, db *sql.DB, it ByHandItem) (int64, string) {
	if it.SlotID > 0 {
		return it.SlotID, ""
	}
	if it.TitleID <= 0 {
		return 0, "pick the movie or show the file is"
	}
	ver := it.VersionID
	if ver <= 0 {
		if db.QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1", it.TitleID).Scan(&ver) != nil {
			return 0, "the title has no Version to fill"
		}
	}
	q := "SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND s.version_id = ? AND i.kind = 'movie'"
	args := []any{it.TitleID, ver}
	if it.Season != nil || it.Episode != nil {
		if it.Season == nil || it.Episode == nil {
			return 0, "pick both the season and the episode"
		}
		q = `SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND s.version_id = ? AND i.kind = 'episode'
			AND json_extract(i.numbers, '$.season') = ? AND json_extract(i.numbers, '$.episode') = ?`
		args = append(args, *it.Season, *it.Episode)
	}
	var id int64
	if db.QueryRowContext(ctx, q+" LIMIT 1", args...).Scan(&id) != nil {
		if it.Season != nil {
			return 0, fmt.Sprintf("the show has no S%02dE%02d in that Version", *it.Season, *it.Episode)
		}
		return 0, "pick the season and episode for a show"
	}
	return id, ""
}

func (im *Importer) importOneByHand(ctx context.Context, db *sql.DB, ns mediafiles.NamingSettings, folders []mediafiles.LibraryFolder, places ByHandPlaces, it ByHandItem, by domain.Actor) (ByHandResult, error) {
	r := ByHandResult{Path: it.Path, Outcome: "refused"}
	refuse := func(why string) (ByHandResult, error) { r.Error = why; return r, nil }
	mode := it.Mode
	if mode == "" {
		mode = "copy"
	}
	if mode != "copy" && mode != "move" {
		return refuse("mode is copy or move")
	}
	src, err := CleanByHandPath(it.Path, places)
	if err != nil {
		return refuse(err.Error())
	}
	r.Path = src
	sfi, err := os.Lstat(src)
	if err != nil || !sfi.Mode().IsRegular() {
		return refuse("not a plain file")
	}
	if !videoExt[strings.ToLower(filepath.Ext(src))] {
		return refuse("only video files are imported by hand for now")
	}
	if libraryFile(ctx, db, folders, src) {
		return refuse("already a file in the Library")
	}
	slotID, why := byHandSlot(ctx, db, it)
	if why != "" {
		return refuse(why)
	}
	r.SlotID = slotID
	var sl mediafiles.Slot
	var item mediafiles.Item
	var ver mediafiles.Version
	var ti mediafiles.Title
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "slots", slotID, &sl); err != nil {
		return refuse("no such Slot")
	}
	for _, g := range []struct {
		table string
		id    int64
		dst   any
	}{{"items", sl.ItemID, &item}, {"versions", sl.VersionID, &ver}} {
		if err := database.Get(ctx, db, g.table, g.id, g.dst); err != nil {
			return r, err
		}
	}
	if err := database.Get(ctx, db, "titles", item.TitleID, &ti); err != nil {
		return r, err
	}
	r.TitleID = ti.ID
	if err := database.Get(ctx, db, "library_folders", ver.LibraryFolderID, &lf); err != nil {
		return r, err
	}
	switch {
	case it.TitleID > 0 && it.TitleID != ti.ID:
		return refuse("the Slot belongs to another title")
	case ti.State == "removed" || sl.State == mediafiles.SlotRemoved:
		return refuse("the title was removed")
	case item.Kind != "movie" && item.Kind != "episode":
		return refuse("music is not imported by hand yet")
	case sl.State == mediafiles.SlotDownloading || sl.State == mediafiles.SlotImporting || sl.State == mediafiles.SlotChosen:
		return refuse("a download is on its way for this; wait for it or remove it first")
	case sl.FileID != nil:
		return refuse("this already has a file; delete it first or pick another Version")
	}
	year := 0
	if ti.Year != nil {
		year = *ti.Year
	}
	p := releasename.Parse(filepath.Base(src), releasename.Hints{Size: sfi.Size(), Show: item.Kind == "episode"})
	folder := safeName(ver.FolderName)
	var target string
	if item.Kind == "movie" {
		if folder == "" || folder == "." || folder == ".." {
			folder = FolderName(ti.Name, year)
		}
		t := Target{TitleName: ti.Name, Year: year, IDs: ti.SourceIDs, Naming: &ns, Edition: ver.Edition}
		target = filepath.Join(lf.Path, folder, t.fileName(p, src))
	} else {
		if folder == "" || folder == "." || folder == ".." {
			folder = mediafiles.ShowFolder(ns, mediafiles.NameFacts{Title: ti.Name, Year: year})
		}
		t := ShowTarget{FolderName: folder, Show: mediafiles.TitleRef{ID: ti.ID, Name: ti.Name, Year: year, Kind: "show"}, IDs: ti.SourceIDs, Order: mediafiles.OrderAired, Naming: &ns}
		if ti.EpisodeOrder != nil && *ti.EpisodeOrder != "" {
			t.Order = *ti.EpisodeOrder
		}
		e := Episode{Ref: episodeRef(item), SlotID: sl.ID}
		if item.Name != nil {
			e.Name = *item.Name
		}
		target = filepath.Join(lf.Path, filepath.FromSlash(t.episodePath(p, src, []Episode{e})))
	}
	if !under(lf.Path, target) || !insideReal(lf.Path, target) {
		return refuse("the new path would leave the Library folder")
	}
	if long := PathTooLong(target); long != "" {
		return refuse(long)
	}
	r.Target = target
	placedBy := "none"
	if filepath.Clean(target) != src {
		ops := im.Ops
		if ops.Link == nil {
			ops.Link = os.Link
		}
		if ops.Copy == nil {
			ops.Copy = copyFile
		}
		res := place(mediafiles.PlanStep{SourceFile: src, TargetPath: target, Method: "hardlink"}, ops, time.Time{})
		if res.Outcome != "placed" {
			r.Outcome, r.Error = res.Outcome, res.Error
			return r, nil
		}
		placedBy = res.PlacedBy
		if mode == "move" {
			if err := os.Remove(src); err != nil {
				r.Error = "placed, but the original could not be removed: " + err.Error()
			} else {
				placedBy = "move"
			}
		}
	}
	r.Outcome, r.PlacedBy = "placed", placedBy
	if err := im.recordByHand(ctx, &r, src, target, lf, ver, item, ti, sl, p, by); err != nil {
		if placedBy == "hardlink" || placedBy == "copy" {
			_ = os.Remove(target)
		}
		return r, err
	}
	return r, nil
}

func (im *Importer) recordByHand(ctx context.Context, r *ByHandResult, src, target string, lf mediafiles.LibraryFolder, ver mediafiles.Version, item mediafiles.Item,
	ti mediafiles.Title, sl mediafiles.Slot, p releasename.Result, by domain.Actor) error {
	rel, err := filepath.Rel(lf.Path, target)
	if err != nil {
		return err
	}
	var size int64
	if fi, err := os.Stat(target); err == nil {
		size = fi.Size()
	}
	facts := InspectFacts(target)
	info := map[string]any{"step": mediafiles.StepFor(p)}
	for k, v := range facts {
		info[k] = v
	}
	release := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	var pr mediafiles.PickRules
	ge := false
	if database.Get(ctx, im.Store.Read(), "pick_rules", ver.PickRulesID, &pr) == nil {
		ge = mediafiles.ReachesGoodEnough(release, pr.Body)
	}
	state, why := mediafiles.SlotInLibrary, "imported by hand"
	if !ge && ver.KeepUpToDate && item.KeepUpToDate {
		state, why = mediafiles.SlotBetterWanted, "imported by hand below good enough, so a better copy is sought"
	}
	step := mediafiles.PlanStep{SourceFile: src, TargetSlotIDs: []int64{sl.ID}, TargetPath: target, Method: "hardlink", Confidence: 1, Why: []string{"chosen by hand"}}
	kind := "movie"
	if item.Kind == "episode" {
		kind = "episodes"
	}
	imp := &mediafiles.Import{FromPath: filepath.Dir(src), Kind: kind, Plan: []mediafiles.PlanStep{step}, Mode: "by_hand", DecidedBy: &by, State: "done",
		Result: []mediafiles.PlaceResult{{SourceFile: src, Outcome: "placed", PlacedBy: r.PlacedBy}}}
	err = im.Store.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: ver.ID, RelativePath: filepath.ToSlash(rel), Size: size, AddedAt: time.Now().UTC(),
			Quality:   mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
			Languages: InspectedLanguages(facts), MediaInfo: info, ReleaseName: &release, Origin: "imported", PlacedBy: r.PlacedBy, Role: "main", State: "present"}
		if g := p.Group.Value; g != "" {
			f.Group = &g
		}
		// A library scan may have noted this file as found, with or without its episodes; that note gives way.
		srcRel := f.RelativePath
		if r, err := filepath.Rel(lf.Path, src); err == nil && under(lf.Path, src) {
			srcRel = filepath.ToSlash(r)
		}
		// A protected note keeps its protection on the imported file.
		protected, err := mediafiles.ProtectedAt(ctx, tx, ver.ID, f.RelativePath, srcRel)
		if err != nil {
			return err
		}
		f.Protected = protected
		if _, err := tx.Exec(`DELETE FROM files WHERE version_id = ? AND relative_path IN (?, ?) AND (slot_ids = '[]' OR origin = 'found')`,
			ver.ID, f.RelativePath, srcRel); err != nil {
			return err
		}
		id, err := database.Insert(tx, "files", f, by)
		if err != nil {
			return err
		}
		r.FileID = id
		if _, err := tx.Exec(`UPDATE slots SET file_id = ?, good_enough = ?, state = ?, last_reason = ?, held_release_id = NULL, active_download_id = NULL,
			updated_at = ?, updated_by = ? WHERE id = ?`, id, ge, string(state), why, database.FormatTime(time.Now()), actorText(by), sl.ID); err != nil {
			return err
		}
		_, err = database.Insert(tx, "imports", imp, by)
		return err
	})
	if err != nil {
		return err
	}
	im.publish("slot", map[string]any{"id": sl.ID, "state": state, "reason": why})
	slotID, titleID := sl.ID, ti.ID
	e := &domain.Event{Actor: by, Kind: "import_done", TitleID: &titleID, SlotID: &slotID, Summary: "Imported by hand: " + filepath.Base(target),
		Data:     map[string]any{"import_id": imp.ID, "by_hand": true, "from": src, "slot_ids": []int64{slotID}, "placed_by": r.PlacedBy, "paths": planPaths(imp.Plan)},
		GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	im.publish("import", imp)
	return nil
}
