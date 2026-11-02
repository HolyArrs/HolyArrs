package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/judge"
	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/parse"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Episode is one episode of a show as an episode import sees it.
type Episode struct {
	Ref    judge.EpisodeRef
	Name   string // the episode name, "" when unknown
	SlotID int64  // its Slot in the Version being filled; 0: none
	// Wanted says this download fills the episode: its Slot is one of the
	// Download's and still waits for it.
	Wanted bool
}

// ShowTarget is where a show download's files belong.
type ShowTarget struct {
	LibraryPath string
	FolderName  string // the show folder (the Version's folder)
	Show        judge.TitleRef
	IDs         map[string]string
	Order       string // aired, daily, absolute
	Episodes    []Episode
	ReleaseName string // the Release's name, the last word on the show
	Naming      *naming.Settings
}

// episodePath is where a file holding eps goes, relative to the Library
// folder, by the naming settings (the same function a rename uses).
func (t ShowTarget) episodePath(p parse.Result, source string, eps []Episode) string {
	ns := naming.Defaults()
	if t.Naming != nil {
		ns = *t.Naming
	}
	ext := filepath.Ext(source)
	release := strings.TrimSuffix(filepath.Base(source), ext)
	f := EpisodeFactsFor(t.Show.Name, t.Show.Year, t.IDs, t.Order, p, release, ext, eps)
	if !ns.RenameOnImport {
		return path.Join(t.FolderName, naming.SeasonFolder(ns, f.Season), naming.CleanWith(ns, release)+strings.ToLower(ext))
	}
	return naming.EpisodePath(ns, t.FolderName, f)
}

// EpisodeFactsFor builds the naming facts of a file holding eps.
func EpisodeFactsFor(show string, year int, ids map[string]string, order string, p parse.Result, release, ext string, eps []Episode) naming.EpisodeFacts {
	f := naming.EpisodeFacts{Facts: naming.FactsFrom(show, year, ids, p, release, ext), Order: order}
	for i, e := range eps {
		if i == 0 {
			f.Season, f.AirDate = e.Ref.Season, e.Ref.AirDay
		}
		f.Episodes = append(f.Episodes, e.Ref.Episode)
		f.Absolute = append(f.Absolute, e.Ref.Absolute)
		f.Titles = append(f.Titles, e.Name)
	}
	return f
}

func labels(eps []Episode) string {
	out := make([]string, len(eps))
	for i, e := range eps {
		out[i] = e.Ref.Label()
	}
	return strings.Join(out, ", ")
}

// PlanEpisodes decides what to do with each file of a show download: each
// video is matched to episodes by its name (S01E02, a multi-episode name,
// an air day or a running number) and placed for the episodes this
// download fills. Files of episodes it does not fill, and files that name
// no episode, are left alone (skip) and reported.
func PlanEpisodes(cands []Candidate, t ShowTarget) []domain.PlanStep {
	t.Show.Kind = "show"
	lib := []judge.TitleRef{t.Show}
	refs := make([]judge.EpisodeRef, len(t.Episodes))
	byItem := map[int64]Episode{}
	for i, e := range t.Episodes {
		refs[i] = e.Ref
		byItem[e.Ref.ItemID] = e
	}
	var steps []domain.PlanStep
	for _, c := range cands {
		ext := strings.ToLower(filepath.Ext(c.Path))
		s := domain.PlanStep{SourceFile: c.Path, TargetSlotIDs: []int64{}, Why: []string{}}
		p := parse.Parse(filepath.Base(c.Path), parse.Hints{Size: c.Size, Show: true})
		switch {
		case programExt[ext] || p.Has("executable") || p.Has("double_extension"):
			s.Method, s.Why = "refuse", []string{"a program file, not a video"}
		case !videoExt[ext]:
			s.Method, s.Why = "skip", []string{"not a video file (" + ext + ")"}
		case isSample(c.Path, p):
			s.Method, s.Why = "refuse", []string{"a sample, not the full video"}
		case !p.IsShow():
			s.Method, s.Why = "skip", []string{"the file name gives no episode numbers; left alone"}
		case judge.IsPack(p):
			s.Method, s.Why = "skip", []string{"the file name names whole seasons, not episodes; left alone"}
		default:
			m := judge.MatchTitle(p, lib)
			if m.Title == nil && strings.TrimSpace(p.Title.Value) == "" {
				// A bare name such as S01E02.mkv: its folder or the Release
				// name says which show it is.
				for _, alt := range []string{filepath.Base(filepath.Dir(c.Path)), t.ReleaseName} {
					if alt == "" {
						continue
					}
					if fm := judge.MatchTitle(parse.Parse(alt, parse.Hints{Show: true}), lib); fm.Title != nil {
						m = fm
						break
					}
				}
			}
			if m.Title == nil {
				s.Method, s.Why = "refuse", []string{"does not match the show: " + m.Detail}
				break
			}
			found, why := judge.MatchEpisodes(p, t.Order, refs)
			if len(found) == 0 {
				s.Method, s.Why = "skip", []string{why + "; left alone"}
				break
			}
			var eps, want []Episode
			for _, f := range found {
				e := byItem[f.ItemID]
				eps = append(eps, e)
				if e.Wanted {
					want = append(want, e)
				}
			}
			if len(want) == 0 {
				s.Method, s.Why = "skip", []string{labels(eps) + " is not filled by this download; left alone"}
				break
			}
			s.Method = "hardlink"
			for _, e := range want {
				s.TargetSlotIDs = append(s.TargetSlotIDs, e.SlotID)
			}
			s.TargetPath = filepath.Join(t.LibraryPath, filepath.FromSlash(t.episodePath(p, c.Path, eps)))
			s.Confidence = confidence(m.Conf)
			s.Why = []string{m.Detail, "holds " + labels(eps)}
			if s.Confidence < MinConfidence {
				s.Method = "refuse"
				s.Why = append(s.Why, "too unsure to import without you")
			}
		}
		steps = append(steps, s)
	}
	// One episode takes one file: the biggest wins, and a file sharing an
	// episode with a bigger one is left alone.
	order := make([]int, 0, len(steps))
	for i := range steps {
		if steps[i].Method == "hardlink" {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		return sizeOf(cands, steps[order[a]].SourceFile) > sizeOf(cands, steps[order[b]].SourceFile)
	})
	taken := map[int64]bool{}
	for _, i := range order {
		clash := false
		for _, id := range steps[i].TargetSlotIDs {
			clash = clash || taken[id]
		}
		if clash {
			steps[i].Method, steps[i].TargetPath = "skip", ""
			steps[i].Why = append(steps[i].Why, "a bigger file in the same download holds the same episode")
			continue
		}
		for _, id := range steps[i].TargetSlotIDs {
			taken[id] = true
		}
	}
	return steps
}

// heldFrom finds where a cut-off run moved an old file: one of the paths
// it recorded that ends with the file's relative path and is still there.
func heldFrom(recorded []string, rel string) string {
	want := "/" + strings.TrimPrefix(filepath.ToSlash(rel), "/")
	for _, r := range recorded {
		s := filepath.ToSlash(r)
		if i := strings.LastIndex(s, "."); i > 0 && strings.HasSuffix(s[:i], want) {
			if _, err := strconv.Atoi(s[i+1:]); err == nil {
				s = s[:i] // holdPath's ".N" for a taken name
			}
		}
		if strings.HasSuffix(s, want) {
			if _, err := os.Lstat(r); err == nil {
				return r
			}
		}
	}
	return ""
}

// showSlot is one Slot of a show Download with what the import did to it.
type showSlot struct {
	sl      domain.Slot
	it      domain.Item
	live    bool   // waits for this download (downloading or importing)
	step    int    // the plan step filling it; -1: none
	lie     string // its file lied: the Release is turned down for it
	outcome string // placed, lied, missing, stuck
	to      domain.SlotState
	reason  string
}

// showImportEntered lets tests see a run of a pack begin.
var showImportEntered func(id int64)

// importShow imports a finished show Download: every episode file goes to
// its episode's Slot, a pack fills many, and each Slot is settled on its
// own (placed, turned down and looked for again, or Needs a decision).
func (im *Importer) importShow(ctx context.Context, d domain.Download, dr domain.Downloader, dryRun bool, by domain.Actor) (*domain.Import, error) {
	if !dryRun {
		// Two runs of one pack never overlap, whatever queued them; the
		// second reads the Download again once the first is done.
		mu, _ := im.runs.LoadOrStore(d.ID, &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()
		if showImportEntered != nil {
			showImportEntered(d.ID)
		}
		if err := store.Get(ctx, im.Store.Read(), "downloads", d.ID, &d); err != nil {
			return nil, err
		}
	}
	db := im.Store.Read()
	slots := map[int64]*showSlot{}
	var ver domain.Version
	var ti domain.Title
	for i, id := range d.SlotIDs {
		ss := &showSlot{step: -1}
		if err := store.Get(ctx, db, "slots", id, &ss.sl); err != nil {
			return nil, err
		}
		if err := store.Get(ctx, db, "items", ss.sl.ItemID, &ss.it); err != nil {
			return nil, err
		}
		if i == 0 {
			if err := store.Get(ctx, db, "versions", ss.sl.VersionID, &ver); err != nil {
				return nil, err
			}
			if err := store.Get(ctx, db, "titles", ss.it.TitleID, &ti); err != nil {
				return nil, err
			}
		}
		if ss.sl.VersionID != ver.ID || ss.it.TitleID != ti.ID {
			return nil, fmt.Errorf("download %d fills Slots of more than one show or Version", d.ID)
		}
		ss.live = ss.sl.State == domain.SlotDownloading || ss.sl.State == domain.SlotImporting
		slots[id] = ss
	}
	liveAny := false
	for _, ss := range slots {
		liveAny = liveAny || ss.live
	}
	if !dryRun && !liveAny {
		return nil, fmt.Errorf("no Slot of download %d waits for it; nothing to import", d.ID)
	}
	var lf domain.LibraryFolder
	if err := store.Get(ctx, db, "library_folders", ver.LibraryFolderID, &lf); err != nil {
		return nil, err
	}
	var pr domain.PickRules
	if err := store.Get(ctx, db, "pick_rules", ver.PickRulesID, &pr); err != nil {
		return nil, err
	}
	ns, err := naming.Read(ctx, db)
	if err != nil {
		return nil, err
	}
	rules, err := ReadFileRules(ctx, db)
	if err != nil {
		return nil, err
	}
	music := ti.Kind == "artist"
	itemWhere := "title_id = ? AND kind = 'episode' AND dropped_at IS NULL"
	args := []any{ti.ID}
	if music {
		// The tracks of the album this download fills: the chosen Pressing's.
		// A track of no album would gather every other loose track.
		part := slots[d.SlotIDs[0]].it.PartID
		if part == nil {
			return nil, fmt.Errorf("download %d: its first track belongs to no album; an album import needs one", d.ID)
		}
		itemWhere = "title_id = ? AND kind = 'track' AND dropped_at IS NULL AND part_id = ?"
		args = append(args, *part)
	}
	items, err := store.Select[domain.Item](ctx, db, "items", itemWhere, args...)
	if err != nil {
		return nil, err
	}
	verSlots, err := store.Select[domain.Slot](ctx, db, "slots", "version_id = ?", ver.ID)
	if err != nil {
		return nil, err
	}
	slotOf := map[int64]int64{}
	for _, s := range verSlots {
		slotOf[s.ItemID] = s.ID
	}
	year := 0
	if ti.Year != nil {
		year = *ti.Year
	}
	relName := ""
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *d.ReleaseID).Scan(&relName)
	}
	t := ShowTarget{LibraryPath: lf.Path, Show: judge.TitleRef{ID: ti.ID, Name: ti.Name, Aliases: ti.Aliases, Year: year, Kind: "show"},
		IDs: ti.SourceIDs, Order: naming.OrderAired, ReleaseName: relName, Naming: &ns}
	if ti.EpisodeOrder != nil && *ti.EpisodeOrder != "" {
		t.Order = *ti.EpisodeOrder
	}
	t.FolderName = safeName(ver.FolderName)
	if t.FolderName == "" || t.FolderName == "." || t.FolderName == ".." {
		t.FolderName = naming.ShowFolder(ns, naming.Facts{Title: ti.Name, Year: year})
	}
	for _, it := range items {
		e := Episode{Ref: judge.EpisodeRef{ItemID: it.ID}, SlotID: slotOf[it.ID]}
		if it.Numbers.Season != nil {
			e.Ref.Season = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.Ref.Episode = *it.Numbers.Episode
		}
		if it.Numbers.Absolute != nil {
			e.Ref.Absolute = *it.Numbers.Absolute
		}
		if it.AirDay != nil {
			e.Ref.AirDay = *it.AirDay
		}
		if it.Name != nil {
			e.Name = *it.Name
		}
		if ss := slots[e.SlotID]; ss != nil && (ss.live || dryRun) {
			e.Wanted = true
		}
		t.Episodes = append(t.Episodes, e)
	}
	root := mapPath(d.SavePath, dr.PathMap)
	cands, bad := candidates(root, d.FilesSeen)
	var plan []domain.PlanStep
	if music {
		at, err := albumTarget(ctx, db, lf, ver, ti, items, slotOf, slots, dryRun, ns)
		if err != nil {
			return nil, err
		}
		plan = PlanTracks(cands, at)
	} else {
		plan = PlanEpisodes(cands, t)
	}
	plan = append(plan, bad...)
	lowSpace, release := im.guard(plan, lf.Path, cands, rules)
	defer release()
	if music {
		wholeAlbumSpace(plan, lowSpace)
	}

	// The copies in place: each step compares its file with them and, when
	// it is no lower, replaces those it wholly covers.
	filled := map[int64]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" {
			for _, id := range st.TargetSlotIDs {
				filled[id] = true
			}
		}
	}
	olds := map[int][]domain.File{} // step -> the Files in place for its Slots
	inspected := map[string]map[string]any{}
	fakes := im.refuseFakes(ctx, db)
	var claimed int64
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(size, 0) FROM releases WHERE id = ?", *d.ReleaseID).Scan(&claimed)
	}
	mismatch := fakes.claimMismatch(claimed, d.FilesSeen)
	for i := range plan {
		if plan[i].Method != "hardlink" {
			continue
		}
		seen := map[int64]bool{}
		for _, id := range plan[i].TargetSlotIDs {
			ss := slots[id]
			if ss == nil || ss.sl.FileID == nil || seen[*ss.sl.FileID] {
				continue
			}
			seen[*ss.sl.FileID] = true
			var f domain.File
			if err := store.Get(ctx, db, "files", *ss.sl.FileID, &f); err == nil && f.State == "present" {
				olds[i] = append(olds[i], f)
			}
		}
		copyWhy := ""
		for _, old := range olds[i] {
			if old.Protected {
				copyWhy = "the copy in place is protected; choose another Version for an upgrade"
			}
		}
		if !music && copyWhy == "" {
			facts := InspectFacts(plan[i].SourceFile)
			inspected[plan[i].SourceFile] = facts
			copyWhy = copyRefusal(ver, filepath.Base(plan[i].SourceFile), relName, "", facts)
		}
		if copyWhy != "" {
			// A copy choice needs review; it does not turn down the Release.
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, copyWhy)
			delete(olds, i)
			continue
		}
		minutes := 0
		for _, id := range plan[i].TargetSlotIDs {
			if ss := slots[id]; ss != nil && ss.it.Runtime != nil && minutes >= 0 {
				minutes += *ss.it.Runtime
			} else {
				minutes = -1 // one length unknown: the floor for an unknown length
			}
		}
		why, err := fakes.fakeFileOf(plan[i].SourceFile, music, max(minutes, 0), relName)
		if err != nil {
			// Not the Release's fault: these Slots wait for a person.
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, unreadable(err))
			continue
		}
		if why == "" {
			why = mismatch
		}
		if why != "" {
			for _, id := range plan[i].TargetSlotIDs {
				slots[id].lie = why
			}
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			continue
		}
		name := strings.TrimSuffix(filepath.Base(plan[i].SourceFile), filepath.Ext(plan[i].SourceFile))
		oldName := ""
		if len(olds[i]) > 0 {
			oldName = fileName(olds[i][0])
		}
		if why := lieOf(name, relName, len(olds[i]) > 0, oldName, pr.Body); why != "" {
			for _, id := range plan[i].TargetSlotIDs {
				slots[id].lie = why
			}
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			delete(olds, i)
			continue
		}
		if len(olds[i]) > 0 {
			id := olds[i][0].ID
			plan[i].ReplacesFileID = &id
		}
	}
	// Only steps still allowed to place cover an old multi-episode file.
	filled = map[int64]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" {
			for _, id := range st.TargetSlotIDs {
				filled[id] = true
			}
		}
	}
	for i, st := range plan {
		for _, id := range st.TargetSlotIDs {
			if ss := slots[id]; ss != nil && st.Method == "hardlink" {
				ss.step = i
			}
		}
	}
	imp := &domain.Import{DownloadID: &d.ID, FromPath: root, Kind: map[bool]string{false: "episodes", true: "tracks"}[music], Plan: plan, Result: []domain.PlaceResult{}, Mode: "dry_run", State: "planned"}
	if dryRun {
		return imp, nil
	}
	imp.Mode = "auto"
	firstSlot := d.SlotIDs[0]
	if lowSpace != nil {
		im.lowSpace(ctx, lowSpace, firstSlot, ti.ID, d.ID, by)
	}

	// A run cut off after placing left an import in placing: reuse it.
	prior, err := store.Select[domain.Import](ctx, db, "imports", "download_id = ? AND state = 'placing' ORDER BY id DESC LIMIT 1", d.ID)
	if err != nil {
		return nil, err
	}
	resumable := map[string]time.Time{}
	recorded := []string{} // where an earlier, cut-off run moved old copies
	if len(prior) == 1 {
		imp.Meta = prior[0].Meta
		for _, r := range prior[0].Result {
			if r.Outcome == "placing" || r.Outcome == "placed" {
				resumable[r.SourceFile] = prior[0].CreatedAt
			}
			if r.HeldAside != "" {
				recorded = append(recorded, r.HeldAside)
			}
			recorded = append(recorded, r.HeldAsideMore...)
		}
	}
	for _, st := range plan {
		r := domain.PlaceResult{SourceFile: st.SourceFile, Outcome: "pending", PlacedBy: "none"}
		if st.Method == "hardlink" {
			r.Outcome = "placing"
		}
		imp.Result = append(imp.Result, r)
	}
	imp.State = "placing"
	var moved []int64
	if err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range d.SlotIDs {
			if ss := slots[id]; ss.live && ss.sl.State == domain.SlotDownloading {
				if err := store.MoveSlotTx(tx, id, domain.SlotImporting, "importing", by); err != nil {
					return err
				}
				moved = append(moved, id)
			}
		}
		if imp.ID > 0 {
			return store.Update(tx, "imports", imp, by)
		}
		_, err := store.Insert(tx, "imports", imp, by)
		return err
	}); err != nil {
		return nil, err
	}
	for _, id := range moved {
		im.publish("slot", map[string]any{"id": id, "state": domain.SlotImporting})
	}
	swept := map[string]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" && !swept[filepath.Dir(st.TargetPath)] {
			swept[filepath.Dir(st.TargetPath)] = true
			sweepStaleParts(filepath.Dir(st.TargetPath))
		}
	}
	// whole says an old File is wholly covered by the files this import
	// places, so it is replaced (moved to the holding folder); one that
	// also holds other episodes stays and lets go of the covered ones.
	whole := func(f domain.File) bool {
		for _, id := range f.SlotIDs {
			if !filled[id] {
				return false
			}
		}
		return true
	}
	abs := func(f domain.File) string { return filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath)) }
	save := func() error {
		return im.Store.Write(ctx, func(tx *sql.Tx) error { return store.Update(tx, "imports", imp, by) })
	}
	// An old File at the very path the new one takes steps aside first,
	// recorded before it moves, and comes back if the new one fails.
	heldFirst := map[int]string{}
	for i, fs := range olds {
		for _, f := range fs {
			if !whole(f) || filepath.Clean(abs(f)) != filepath.Clean(plan[i].TargetPath) {
				continue
			}
			if r := heldFrom(recorded, f.RelativePath); r != "" {
				heldFirst[i] = r
				continue
			}
			if _, err := os.Lstat(abs(f)); err != nil {
				continue
			}
			to, err := holdPath(lf.Path, f.RelativePath, time.Now())
			if err != nil {
				return imp, fmt.Errorf("moving the old copy aside: %w", err)
			}
			imp.Result[i].HeldAside = to
			if err := save(); err != nil {
				return imp, err
			}
			if err := moveFile(abs(f), to); err != nil {
				return imp, fmt.Errorf("moving the old copy aside: %w", err)
			}
			heldFirst[i] = to
		}
	}
	if crashAt != nil && len(heldFirst) > 0 {
		if err := crashAt("held"); err != nil {
			return imp, err
		}
	}
	imp.Result = execute(plan, im.Ops, resumable)
	for i, to := range heldFirst {
		imp.Result[i].HeldAside = to
	}
	if crashAt != nil {
		if err := crashAt("placed"); err != nil {
			return imp, err
		}
	}
	// replacedTo is where each replaced File went ("" when it was gone).
	replacedTo := map[int64]string{}
	for i, fs := range olds {
		placed := imp.Result[i].Outcome == "placed"
		for _, f := range fs {
			if !whole(f) {
				continue
			}
			sameSpot := filepath.Clean(abs(f)) == filepath.Clean(plan[i].TargetPath)
			switch {
			case sameSpot && !placed:
				if to := heldFirst[i]; to != "" {
					if err := moveFile(to, abs(f)); err != nil {
						log.Printf("importer: putting the old copy back from %s: %v", to, err)
					} else {
						imp.Result[i].HeldAside = ""
					}
				}
			case sameSpot:
				replacedTo[f.ID] = heldFirst[i]
			case placed:
				to := heldFrom(recorded, f.RelativePath)
				if _, err := os.Lstat(abs(f)); err == nil && to == "" {
					var herr error
					if to, herr = holdPath(lf.Path, f.RelativePath, time.Now()); herr == nil {
						imp.Result[i].HeldAsideMore = append(imp.Result[i].HeldAsideMore, to)
						if herr = save(); herr == nil {
							herr = moveFile(abs(f), to)
						}
					}
					if herr != nil {
						to = ""
						log.Printf("importer: moving the replaced copy aside: %v", herr)
						e := &domain.Event{Actor: by, Kind: "file_hold_failed", SlotID: &firstSlot, Summary: "Could not move the replaced copy aside: " + herr.Error(),
							Data: map[string]any{"file_id": f.ID, "path": f.RelativePath}, GroupKey: "download:" + strconv.FormatInt(d.ID, 10)}
						if im.Store.AddEvent(ctx, e) == nil {
							im.publish("event", e)
						}
					}
				}
				replacedTo[f.ID] = to
			}
		}
	}
	if crashAt != nil && len(replacedTo) > 0 {
		if err := crashAt("held_late"); err != nil {
			return imp, err
		}
	}

	// Settle every Slot of the download on its own.
	matched := false
	for _, st := range plan {
		matched = matched || len(st.TargetSlotIDs) > 0
	}
	for _, ss := range slots {
		if !ss.live {
			continue
		}
		switch {
		case ss.step >= 0 && imp.Result[ss.step].Outcome == "placed":
			ss.outcome = "placed"
		case ss.lie != "":
			ss.outcome, ss.reason = "lied", ss.lie
		case ss.step >= 0:
			ss.outcome = "stuck"
			ss.reason = "the file could not be placed: " + imp.Result[ss.step].Error
		case matched:
			// The download held other episodes but not this one, or only a
			// file that could not be used for it.
			ss.outcome, ss.reason = "missing", "the download did not hold "+refLabel(ss.it)
			for i, st := range plan {
				if slices.Contains(st.TargetSlotIDs, ss.sl.ID) {
					ss.outcome, ss.reason = "stuck", "the file for "+refLabel(ss.it)+" could not be placed: "+strings.Join(plan[i].Why, "; ")
				}
			}
		default:
			ss.outcome, ss.reason = "stuck", "nothing in the download could be matched to an episode; look at the import plan"
			if music {
				ss.reason = "nothing in the download could be matched to a track; look at the import plan"
			}
			for _, r := range imp.Result {
				if strings.HasPrefix(r.Error, "not enough free space") {
					ss.reason = "nothing could be placed: not enough free space in the Library folder"
				}
			}
		}
	}
	kept := func(ss *showSlot) bool { return ver.KeepUpToDate && ss.it.KeepUpToDate && ti.KeepUpToDate }
	placed, failed := 0, 0
	for _, r := range imp.Result {
		switch r.Outcome {
		case "placed":
			placed++
		case "failed":
			failed++
		}
	}
	var lookAgain []int64
	newFiles := map[int64]int64{} // old File -> the File that replaced it
	now := time.Now().UTC()
	err = im.Store.Write(ctx, func(tx *sql.Tx) error {
		// Replaced Files are marked first; one moved to the holding folder
		// takes its new path along, which frees the old one.
		for i, fs := range olds {
			if imp.Result[i].Outcome != "placed" {
				continue
			}
			for _, f := range fs {
				if !whole(f) {
					var rest []int64
					for _, id := range f.SlotIDs {
						if !filled[id] {
							rest = append(rest, id)
						}
					}
					if _, err := tx.Exec("UPDATE files SET slot_ids = ?, updated_at = ?, updated_by = ? WHERE id = ?",
						jsonIDs(rest), store.FormatTime(now), actorText(by), f.ID); err != nil {
						return err
					}
					continue
				}
				rel := f.RelativePath
				if to := replacedTo[f.ID]; to != "" {
					if rp, err := filepath.Rel(lf.Path, to); err == nil {
						rel = filepath.ToSlash(rp)
					}
				} else {
					rel += "#replaced-" + strconv.FormatInt(f.ID, 10)
				}
				if _, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
					rel, store.FormatTime(now), actorText(by), f.ID); err != nil {
					return err
				}
			}
		}
		for i, r := range imp.Result {
			if r.Outcome != "placed" {
				continue
			}
			step := plan[i]
			rel, err := filepath.Rel(lf.Path, step.TargetPath)
			if err != nil {
				return err
			}
			var size int64
			if fi, err := os.Stat(step.TargetPath); err == nil {
				size = fi.Size()
			}
			p := parse.Parse(filepath.Base(step.SourceFile), parse.Hints{Show: !music})
			name := strings.TrimSuffix(filepath.Base(step.SourceFile), filepath.Ext(step.SourceFile))
			if music && relName != "" {
				// A track's file name says nothing of its quality: the
				// Release name does (FLAC, 320, V0).
				name = relName
			}
			f := &domain.File{SlotIDs: step.TargetSlotIDs, VersionID: ver.ID, RelativePath: filepath.ToSlash(rel), Size: size, AddedAt: now,
				Quality:   domain.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
				Languages: p.Languages, MediaInfo: map[string]any{"step": judge.StepFor(p)}, ReleaseName: &name, Origin: "imported", PlacedBy: r.PlacedBy,
				FellBack: r.FellBack, Role: "main", State: "present"}
			for k, value := range inspected[step.SourceFile] {
				f.MediaInfo[k] = value
			}
			if !music {
				f.Languages = InspectedLanguages(inspected[step.SourceFile])
			}
			if music {
				f.Quality = domain.Quality{Source: strings.TrimPrefix(strings.ToLower(filepath.Ext(step.SourceFile)), ".")}
			}
			if f.Languages == nil {
				f.Languages = []string{}
			}
			if g := p.Group.Value; g != "" {
				f.Group = &g
			}
			id, err := store.Insert(tx, "files", f, by)
			if err != nil {
				return err
			}
			for _, o := range olds[i] {
				newFiles[o.ID] = id
			}
			how := r.PlacedBy
			if r.FellBack {
				how += " (hardlink not possible)"
			}
			ge := judge.ReachesGoodEnough(name, pr.Body)
			for _, sid := range step.TargetSlotIDs {
				ss := slots[sid]
				if _, err := tx.Exec("UPDATE slots SET file_id = ?, active_download_id = NULL, good_enough = ? WHERE id = ?", id, ge, sid); err != nil {
					return err
				}
				ss.to, ss.reason = domain.SlotInLibrary, "placed by "+how
				if len(olds[i]) > 0 {
					ss.reason += "; replaced " + fileName(olds[i][0])
				}
				if err := store.MoveSlotTx(tx, sid, ss.to, ss.reason, by); err != nil {
					return err
				}
				if !ge && kept(ss) {
					ss.to = domain.SlotBetterWanted
					if err := store.MoveSlotTx(tx, sid, ss.to, ss.reason+"; below good enough, so a better copy is sought", by); err != nil {
						return err
					}
				}
			}
		}
		var downFor []int64
		for _, id := range d.SlotIDs {
			ss := slots[id]
			if !ss.live || ss.outcome == "placed" {
				continue
			}
			hasFile := ss.sl.FileID != nil
			switch ss.outcome {
			case "lied", "missing":
				// The Release is turned down for this episode only, and the
				// episode looks again.
				downFor = append(downFor, id)
				if _, err := tx.Exec("UPDATE slots SET active_download_id = NULL WHERE id = ?", id); err != nil {
					return err
				}
				switch {
				case ss.outcome == "lied" && !hasFile:
					ss.to = domain.SlotRejected
				case ss.outcome == "lied":
					ss.to = domain.SlotInLibrary
				case hasFile:
					ss.to = domain.SlotBetterWanted
				default:
					ss.to = domain.SlotWanted
				}
				if err := store.MoveSlotTx(tx, id, ss.to, ss.reason, by); err != nil {
					return err
				}
				if ss.to == domain.SlotInLibrary && kept(ss) && !ss.sl.GoodEnough {
					ss.to = domain.SlotBetterWanted
					if err := store.MoveSlotTx(tx, id, ss.to, ss.reason+"; the copy in place stays and a better one is sought", by); err != nil {
						return err
					}
				}
				if ss.to != domain.SlotInLibrary {
					lookAgain = append(lookAgain, id)
				}
			default:
				ss.to = domain.SlotNeedsADecision
				if err := store.MoveSlotTx(tx, id, ss.to, ss.reason, by); err != nil {
					return err
				}
			}
		}
		if len(downFor) > 0 && d.ReleaseID != nil {
			if err := turnDownForTx(tx, *d.ReleaseID, downFor, by); err != nil {
				return err
			}
		}
		switch {
		case placed > 0 && failed == 0 && len(downFor) == 0 && !anyStuck(slots):
			imp.State = "done"
		case placed > 0:
			imp.State = "partly_done"
		case len(downFor) > 0 && !anyStuck(slots):
			imp.State = "rejected"
		default:
			imp.State = "failed"
		}
		return store.Update(tx, "imports", imp, by)
	})
	if err != nil {
		return imp, err
	}
	titleID, dlID := ti.ID, d.ID
	group := "download:" + strconv.FormatInt(d.ID, 10)
	for _, id := range d.SlotIDs {
		ss := slots[id]
		if ss.live && ss.to != "" {
			im.publish("slot", map[string]any{"id": id, "state": ss.to, "reason": ss.reason})
		}
		if ss.outcome == "lied" || ss.outcome == "missing" {
			slotID := id
			e := &domain.Event{Actor: by, Kind: "import_rejected", TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID,
				Summary: "Import rejected for " + refLabel(ss.it) + ": " + ss.reason,
				Data:    map[string]any{"import_id": imp.ID, "release_id": d.ReleaseID, "reason": ss.reason}, GroupKey: "slot:" + strconv.FormatInt(id, 10)}
			if im.Store.AddEvent(ctx, e) == nil {
				im.publish("event", e)
			}
		}
	}
	var placedLabels []string
	placedIDs := []int64{}
	for _, id := range d.SlotIDs {
		if slots[id].outcome == "placed" {
			placedLabels = append(placedLabels, refLabel(slots[id].it))
			placedIDs = append(placedIDs, id)
		}
	}
	summary := fmt.Sprintf("Import %s: %d placed", imp.State, placed)
	if len(placedLabels) > 0 {
		summary += " (" + strings.Join(placedLabels, ", ") + ")"
	}
	e := &domain.Event{Actor: by, Kind: "import_" + imp.State, TitleID: &titleID, SlotID: &firstSlot, DownloadID: &dlID, Summary: summary,
		Data: map[string]any{"import_id": imp.ID, "result": imp.Result, "from": imp.FromPath, "paths": planPaths(imp.Plan), "slot_ids": d.SlotIDs,
			"placed_slot_ids": placedIDs}, GroupKey: group}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	for oldID, newID := range newFiles {
		var f domain.File
		if store.Get(ctx, im.Store.Read(), "files", oldID, &f) != nil || f.State != "replaced" {
			continue
		}
		e := &domain.Event{Actor: by, Kind: "file_replaced", TitleID: &titleID, SlotID: &firstSlot, DownloadID: &dlID,
			Summary: "Replaced " + fileName(f) + " with a better copy", Data: map[string]any{"old_file_id": oldID, "new_file_id": newID, "moved_to": replacedTo[oldID]},
			GroupKey: group}
		im.upgradeFacts(ctx, e.Data, oldID, newID)
		if im.Store.AddEvent(ctx, e) == nil {
			im.publish("event", e)
		}
	}
	im.publish("import", imp)
	if im.OnRejected != nil {
		for _, id := range lookAgain {
			im.OnRejected(ctx, id)
		}
	}
	return imp, nil
}

func anyStuck(slots map[int64]*showSlot) bool {
	for _, ss := range slots {
		if ss.live && ss.outcome == "stuck" {
			return true
		}
	}
	return false
}

// refLabel names an episode the way people write it: S01E02.
func refLabel(it domain.Item) string {
	if it.Kind == "track" {
		d, n := 1, 0
		if it.Numbers.Disc != nil {
			d = *it.Numbers.Disc
		}
		if it.Numbers.Track != nil {
			n = *it.Numbers.Track
		}
		return trackLabel(d, n, map[bool]int{false: 1, true: 2}[d > 1])
	}
	s, e := 0, 0
	if it.Numbers.Season != nil {
		s = *it.Numbers.Season
	}
	if it.Numbers.Episode != nil {
		e = *it.Numbers.Episode
	}
	return fmt.Sprintf("S%02dE%02d", s, e)
}

// fileName is the name a File's copy is known by: its release name, else
// its file name.
func fileName(f domain.File) string {
	if f.ReleaseName != nil && *f.ReleaseName != "" {
		return *f.ReleaseName
	}
	return strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath))
}

func jsonIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// turnDownForTx turns a Release down for some episodes only: the Release
// stays on the Turned down list with them and is still judged for the
// others (the same record a turn down by hand for one episode writes).
func turnDownForTx(tx *sql.Tx, releaseID int64, slotIDs []int64, by domain.Actor) error {
	var cur sql.NullString
	if err := tx.QueryRow("SELECT json_extract(verdict, '$.turned_down_slot_ids') FROM releases WHERE id = ?", releaseID).Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	ids := []int64{}
	if cur.Valid {
		for _, s := range strings.Split(strings.Trim(cur.String, "[]"), ",") {
			if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && !slices.Contains(ids, n) {
				ids = append(ids, n)
			}
		}
	}
	for _, id := range slotIDs {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	_, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL,
		verdict = json_set(json_set(COALESCE(verdict, '{}'), '$.turned_down_slot_ids', json(?)), '$.turned_down_reason', ?), updated_at = ?, updated_by = ? WHERE id = ?`,
		jsonIDs(ids), "the files did not fill these episodes", store.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}
