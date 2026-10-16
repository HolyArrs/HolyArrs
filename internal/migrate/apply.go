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
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/alerts"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/subtitles"
)

// KindMove is the Job that runs an approved move; KindUndo undoes one.
const (
	KindMove = "migrate_move"
	KindUndo = "migrate_undo"
)

// Results a report line can have. The dry run says what would happen
// (create, adopt, change); the move says what did (created, adopted,
// changed).
const (
	ResCreate   = "create"
	ResCreated  = "created"
	ResAdopt    = "adopt"
	ResAdopted  = "adopted"
	ResChange   = "change"
	ResChanged  = "changed"
	ResMatched  = "matched"   // mapped onto something already here
	ResAlready  = "already"   // already here (or moved before): nothing added
	ResSkipped  = "skipped"   // its group was left out, or its part is off
	ResNotFound = "not_found" // its folder is not on this server
	ResFailed   = "failed"
	ResRemoved  = "removed" // undo
	ResKept     = "kept"    // undo: left in place, with the reason
)

// Line is one item's result.
type Line struct {
	Group  string `json:"group"`
	Key    string `json:"key"`
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
	Needs  string `json:"needs,omitempty"`
}

// Report is what a dry run, a move or an undo did.
type Report struct {
	Move       int64          `json:"move,omitempty"`
	Dry        bool           `json:"dry"`
	Undo       bool           `json:"undo,omitempty"`
	Apps       []string       `json:"apps"`
	At         time.Time      `json:"at"`
	Lines      []Line         `json:"lines"`
	Counts     map[string]int `json:"counts"`
	NotMoved   []NotMoved     `json:"not_moved"`
	NeedsInput []string       `json:"needs_input"`
	Notes      []string       `json:"notes,omitempty"`
	Next       []string       `json:"next"`
}

// Mover runs moves. It never moves, renames or deletes a media file and
// never contacts or writes to the old apps: it works from kept Snapshots.
type Mover struct {
	Store   *store.Store
	Library *library.Service
	Queue   *jobs.Queue
	// PartOn says whether a part is on (movies, shows, music, subtitles,
	// wishlist); nil counts every part on.
	PartOn func(part string) bool
	Now    func() time.Time
}

// Register handles the move and undo Jobs.
func (m *Mover) Register(q *jobs.Queue) {
	q.Handle(KindMove, m.handle)
	q.Handle(KindUndo, m.handleUndo)
}

func (m *Mover) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Mover) partOn(p string) bool { return m.PartOn == nil || m.PartOn(p) }

// Errors the move routes answer with; their text is safe to show.
var (
	ErrNoMove   = errors.New("no such move")
	ErrMoveBusy = errors.New("this move is still running")
	ErrUndone   = errors.New("this move was already undone")
	// ErrNoSnapshotPicked asks for at least one snapshot.
	ErrNoSnapshotPicked = errors.New("pick at least one read app")
	ErrTooManySnaps     = errors.New("pick at most 12 read apps")
	// ErrPlanChanged: the move must start from the plan its dry run showed.
	ErrPlanChanged = errors.New("the plan changed since its dry run; dry run again")
)

// LoadPlan builds the Plan for kept Snapshots.
func LoadPlan(ctx context.Context, st *store.Store, ids []int64, ch Choices) (*Plan, error) {
	if len(ids) == 0 {
		return nil, ErrNoSnapshotPicked
	}
	if len(ids) > 12 {
		return nil, ErrTooManySnaps
	}
	if err := ValidRules(ch.PathRules); err != nil {
		return nil, err
	}
	var in []Input
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		_, s, err := Get(ctx, st, id)
		if err != nil {
			return nil, err
		}
		in = append(in, Input{ID: id, Snap: s})
	}
	return BuildPlan(in, ch), nil
}

// SuggestRules suggests a path rule for every old library folder that is
// not on this server as it is: to a Library folder here with the same last
// name, or to a folder of that name directly under one here.
func SuggestRules(ctx context.Context, st *store.Store, p *Plan) []PathRule {
	var here []string
	rows, err := st.Read().QueryContext(ctx, "SELECT path FROM library_folders ORDER BY id")
	if err == nil {
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil {
				here = append(here, s)
			}
		}
		rows.Close()
	}
	out := []PathRule{}
	for _, old := range p.Paths {
		if isDir(filepath.FromSlash(old)) {
			continue
		}
		base := strings.ToLower(old[strings.LastIndexByte(old, '/')+1:])
		for _, h := range here {
			if strings.ToLower(filepath.Base(h)) == base {
				out = append(out, PathRule{From: old, To: filepath.ToSlash(h)})
				break
			}
		}
	}
	return out
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// partOf is the part an item belongs to ("" for the core).
func partOf(it Item) string {
	kind := ""
	switch {
	case it.Folder != nil:
		kind = it.Folder.Kind
	case it.Title != nil:
		kind = it.Title.Kind
	case it.NeverAdd != nil:
		kind = it.NeverAdd.Kind
	case it.Subtitles != nil:
		return "subtitles"
	case it.Ask != nil:
		return "wishlist"
	}
	return map[string]string{"movie": "movies", "show": "shows", "music": "music", "artist": "music"}[kind]
}

// run is one pass over a Plan.
type run struct {
	m      *Mover
	ctx    context.Context
	move   int64
	dry    bool
	by     domain.Actor
	rep    *Report
	people map[string]int64
	owner  int64
}

// Run dry-runs (dry) or applies a Plan. progress, when set, hears how far
// it is.
func (m *Mover) Run(ctx context.Context, moveID int64, p *Plan, dry bool, by domain.Actor, progress func(done, total int)) *Report {
	r := &run{m: m, ctx: ctx, move: moveID, dry: dry, by: by, people: map[string]int64{},
		rep: &Report{Move: moveID, Dry: dry, Apps: p.Apps, At: m.now(), Lines: []Line{}, Counts: map[string]int{},
			NotMoved: append([]NotMoved{}, p.NotMoved...), NeedsInput: append([]string{}, p.NeedsInput...), Notes: p.Notes}}
	_ = m.Store.Read().QueryRowContext(ctx, "SELECT id FROM users WHERE role = 'owner'").Scan(&r.owner)
	if r.owner == 0 {
		r.owner = by.ID
	}
	total := 0
	for _, g := range p.Groups {
		total += len(g.Items)
	}
	done := 0
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if ctx.Err() != nil {
				r.line(g.Name, it, ResFailed, "the move was stopped before this", "")
				continue
			}
			var res, detail string
			switch {
			case g.Skipped:
				res, detail = ResSkipped, "you left "+strings.ToLower(GroupLabels[g.Name])+" out"
			case partOf(it) != "" && !m.partOn(partOf(it)):
				res, detail = ResSkipped, "its part is off"
			default:
				res, detail = r.one(g.Name, it)
			}
			needs := ""
			if res == ResCreated || res == ResCreate {
				needs = it.Needs
			}
			r.line(g.Name, it, res, detail, needs)
			done++
			if progress != nil && (done%25 == 0 || done == total) {
				progress(done, total)
			}
		}
	}
	r.rep.Next = next(r.rep)
	return r.rep
}

func (r *run) line(group string, it Item, res, detail, needs string) {
	r.rep.Lines = append(r.rep.Lines, Line{Group: group, Key: it.Key, Name: it.Name, Result: res, Detail: detail, Needs: needs})
	r.rep.Counts[res]++
	if needs != "" {
		r.rep.NeedsInput = append(r.rep.NeedsInput, GroupLabels[group]+" \""+it.Name+"\": "+needs)
	}
}

func next(rep *Report) []string {
	var out []string
	if rep.Counts[ResNotFound] > 0 {
		out = append(out, "Some folders were not found on this server: add path rules and move again (nothing is added twice).")
	}
	if len(rep.NeedsInput) > 0 {
		out = append(out, "Type the keys and passwords listed under \"needs your input\": Sources, Downloaders, Alerts and subtitle sites stay paused until then.")
	}
	links, people, same := false, false, false
	for _, l := range rep.Lines {
		made := l.Result == ResCreated || l.Result == ResCreate
		switch {
		case (l.Group == GroupSources || l.Group == GroupDownloaders) && made:
			links = true
		case l.Group == GroupPeople && made:
			people = true
		case l.Group == GroupPeople && l.Result == ResSkipped && strings.Contains(l.Detail, "same name"):
			same = true
		}
	}
	if links {
		out = append(out, "Moved Sources and Downloaders are always added paused and cannot reach your home network. Check each one under Settings > Sources and Settings > Downloaders, turn on \"reach the local network\" only for one that lives on your network, then resume it.")
	}
	if people {
		out = append(out, "People came over as turned-off member placeholders (admin or owner only where you confirmed it): invite them from Settings > Users.")
	}
	if same {
		out = append(out, "Some people have the same name as someone here and were left out: choose merge or skip for each on the move screen, dry run again and move again.")
	}
	if rep.Counts[ResFailed] > 0 {
		out = append(out, "Some items failed; their reason is on their line. Fix it and move again.")
	}
	if len(rep.NotMoved) > 0 {
		out = append(out, "Look over the not-moved list: those settings have no HolyArrs match and stay in the old apps.")
	}
	out = append(out, "Keep the old apps until HolyArrs runs the way you want; this move changed nothing in them.")
	return out
}

// made says whether a live (not undone) move already made key, and its row.
func (r *run) made(key string) (int64, bool) {
	var id int64
	err := r.m.Store.Read().QueryRowContext(r.ctx, `SELECT m.row_id FROM move_made m JOIN moves v ON v.id = m.move_id
		WHERE m.key = ? AND v.state != 'undone' ORDER BY m.rowid DESC LIMIT 1`, key).Scan(&id)
	return id, err == nil
}

// natural names, per kind of marked row, its table and the column that
// with created_at fingerprints it: SQLite gives a deleted row's id to the
// next row, so undo deletes a row only while its fingerprint still holds.
var natural = map[string][2]string{
	"ask": {"asks", "name"}, "user": {"users", "name"}, "alert": {"alerts", "name"}, "subtitle_site": {"subtitle_sites", "name"},
	"source": {"sources", "name"}, "downloader": {"downloaders", "name"}, "never_add": {"never_add", "name"},
	"title": {"titles", "kind"}, "folder": {"library_folders", "path"},
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// fingerprint reads a marked row's created_at and natural key ("" when the
// row is gone or its kind has none).
func fingerprint(ctx context.Context, q rowQuerier, what string, id int64) string {
	n, ok := natural[what]
	if !ok {
		return ""
	}
	var fp string
	_ = q.QueryRowContext(ctx, "SELECT created_at || char(31) || "+n[1]+" FROM "+n[0]+" WHERE id = ?", id).Scan(&fp)
	return fp
}

// markTx records, in the transaction that made the row, that this move
// made it, with its fingerprint; before and after are a changed setting's
// old and new values.
func (r *run) markTx(tx *sql.Tx, key, what string, id int64, before, after string) error {
	fp := ""
	if id != 0 {
		if fp = fingerprint(r.ctx, tx, what, id); fp == "" {
			return errors.New("the new row was not found to mark")
		}
	}
	_, err := tx.Exec(`INSERT OR REPLACE INTO move_made (move_id, key, what, row_id, before, after, fingerprint, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.move, key, what, id, before, after, fp, store.FormatTime(r.m.now()))
	return err
}

// markWith is markTx as an after-insert step of another package's save.
func (r *run) markWith(key, what string) func(tx *sql.Tx, id int64) error {
	return func(tx *sql.Tx, id int64) error { return r.markTx(tx, key, what, id, "", "") }
}

func (r *run) one(group string, it Item) (string, string) {
	if id, ok := r.made(it.Key); ok && group != GroupRules {
		if it.Person != nil {
			r.people[it.Key] = id
		}
		return ResAlready, "moved before"
	}
	switch {
	case it.Folder != nil:
		return r.folder(it)
	case it.Rules != nil:
		return r.rules(it)
	case it.Title != nil:
		return r.title(it)
	case it.NeverAdd != nil:
		return r.neverAdd(it)
	case it.Source != nil:
		return r.source(it)
	case it.Downloader != nil:
		return r.downloader(it)
	case it.Naming != nil:
		return r.naming(it)
	case it.Subtitles != nil:
		return r.subtitles(it)
	case it.Person != nil:
		return r.person(it)
	case it.Ask != nil:
		return r.ask(it)
	case it.Alert != nil:
		return r.alert(it)
	}
	return ResFailed, "nothing to do for it"
}

func (r *run) id(q string, args ...any) int64 {
	var id int64
	_ = r.m.Store.Read().QueryRowContext(r.ctx, q, args...).Scan(&id)
	return id
}

// result is a made row's line: failed with the reason, or res. The row and
// its mark are made in one transaction, so a failed one left nothing.
func result(err error, res, detail string) (string, string) {
	if err != nil {
		return ResFailed, err.Error()
	}
	return res, detail
}

func (r *run) folder(it Item) (string, string) {
	f := it.Folder
	p := filepath.FromSlash(f.Path)
	if r.id("SELECT id FROM library_folders WHERE path = ?", filepath.Clean(p)) != 0 {
		return ResAlready, "already a Library folder"
	}
	if !filepath.IsAbs(p) || !isDir(p) {
		return ResNotFound, "no folder at " + f.Path + " on this server; add a path rule"
	}
	if r.dry {
		return ResCreate, "a " + f.Kind + " Library folder at " + f.Path
	}
	paths, _ := library.DownloadPaths(r.ctx, r.m.Store.Read(), r.m.Library.SavePath)
	_, err := library.AddFolder(r.ctx, r.m.Store, library.FolderInput{Path: p, Kind: f.Kind, Label: f.Label}, paths, r.by, r.markWith(it.Key, "folder"))
	return result(err, ResCreated, "a "+f.Kind+" Library folder at "+f.Path)
}

func (r *run) presetID(name, titleKind string) int64 {
	if name == "" {
		return 0
	}
	music := `kinds LIKE '%"music"%'`
	if titleKind != "artist" {
		music = `kinds NOT LIKE '%"music"%'`
	}
	return r.id("SELECT id FROM pick_rules WHERE name = ? AND "+music, name)
}

func (r *run) rules(it Item) (string, string) {
	kind := "movie"
	if it.Rules.App == string(Lidarr) {
		kind = "artist"
	}
	if r.presetID(it.Rules.Preset, kind) == 0 {
		return ResNotFound, "the Pick rules " + it.Rules.Preset + " are not here any more; titles keep the default"
	}
	return ResMatched, "titles from it use " + it.Rules.Preset
}

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
		return ResAlready, "already in the Library"
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
		return ResAdopt, "adopted in place from " + filepath.ToSlash(dir) + "; no file is moved or renamed"
	}
	lfID := r.id("SELECT id FROM library_folders WHERE path = ?", filepath.Clean(lib))
	if lfID == 0 {
		return ResNotFound, "its Library folder " + t.Library + " is not here (not moved, or left out)"
	}
	c := library.AdoptChoice{Folder: t.Folder, ID: t.ID}
	if t.Kind == "movie" {
		c = library.AdoptChoice{Folder: t.Folder, TMDB: strings.TrimPrefix(t.ID, "tmdb:")}
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
		_, _ = r.m.Library.Remove(context.WithoutCancel(r.ctx), id, library.RemoveChoice{}, true, r.by)
		return ResFailed, "adopted, but its mark was not kept, so it was taken out again: " + mk.Error()
	}
	pr := r.presetID(t.Preset, t.Kind)
	err = r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET keep_up_to_date = ? WHERE id = ?", t.KeepUpToDate, id); err != nil {
			return err
		}
		if pr != 0 {
			_, err := tx.Exec("UPDATE versions SET pick_rules_id = ? WHERE title_id = ?", pr, id)
			return err
		}
		return nil
	})
	if err != nil {
		return ResFailed, "adopted, but its choices were not kept: " + err.Error()
	}
	for _, n := range t.SeasonsOff {
		_ = r.m.Library.SetSeason(r.ctx, id, n, false, r.by) // a season the show does not have is fine
	}
	d := "adopted in place from " + filepath.ToSlash(dir)
	if res[0].File != "" {
		d += " with " + res[0].File
	}
	if !t.KeepUpToDate {
		d += "; Keep up to date off"
	}
	return ResAdopted, d
}

func (r *run) neverAdd(it Item) (string, string) {
	n := it.NeverAdd
	db := r.m.Store.Read()
	var on bool
	switch n.Kind {
	case "movie":
		on, _ = library.OnNeverAdd(r.ctx, db, n.IDs["tmdb"])
	case "show":
		on, _ = library.ShowOnNeverAdd(r.ctx, db, n.IDs)
	default:
		on, _ = library.ArtistOnNeverAdd(r.ctx, db, n.IDs["musicbrainz"])
	}
	if on {
		return ResAlready, "already on the Never-add list"
	}
	if r.dry {
		return ResCreate, "added to the Never-add list"
	}
	row := domain.NeverAdd{Kind: n.Kind, SourceIDs: n.IDs, Name: it.Name, Reason: "moved from the old app's exclusions"}
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		var err error
		if id, err = store.Insert(tx, "never_add", &row, r.by); err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "never_add", id, "", "")
	})
	return result(err, ResCreated, "added to the Never-add list")
}

// A moved Source or Downloader is always created paused and without
// local-network reach, whatever its keys: the owner checks each one, turns
// on "reach the local network" where it belongs, and resumes it. The
// link-local and metadata refusal of the dial policies stays either way.
func (r *run) source(it Item) (string, string) {
	s := it.Source
	if r.id("SELECT id FROM sources WHERE name = ?", it.Name) != 0 {
		return ResAlready, "a Source with this name is here"
	}
	what := s.Format + " at " + s.URL + ", paused until you check and resume it"
	if r.dry {
		return ResCreate, what
	}
	local := false
	prio := s.Priority
	in := source.Input{Name: it.Name, Format: s.Format, Protocol: s.Protocol, URL: s.URL, Kinds: s.Kinds, Categories: s.Categories,
		MinSeeders: s.MinSeeders, Priority: &prio, AllowLocal: &local}
	_, err := source.Add(r.ctx, r.m.Store, nil, in, r.by, func(tx *sql.Tx, id int64) error {
		if _, err := tx.Exec("UPDATE sources SET state = 'disabled' WHERE id = ?", id); err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "source", id, "", "")
	})
	return result(err, ResCreated, what)
}

func (r *run) downloader(it Item) (string, string) {
	d := it.Downloader
	if r.id("SELECT id FROM downloaders WHERE url = ? AND removed = 0", d.URL) != 0 {
		return ResAlready, "a Downloader at this address is here"
	}
	name := it.Name
	if r.id("SELECT id FROM downloaders WHERE name = ?", name) != 0 {
		name = clip(name+" (moved)", 80)
	}
	what := d.Adapter + " at " + d.URL + ", paused until you check and resume it; it downloads into its own holyarrs category"
	if r.dry {
		return ResCreate, what
	}
	proto := "torrent"
	if d.Adapter == "sabnzbd" {
		proto = "usenet"
	}
	row := domain.Downloader{Name: name, Adapter: d.Adapter, Protocol: proto, URL: d.URL, Category: "holyarrs", Priority: d.Priority,
		PathMap: []domain.PathMap{}, AfterImport: "leave", State: "disabled", Username: d.Username, AllowLocal: false}
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		id, err := store.Insert(tx, "downloaders", &row, r.by)
		if err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "downloader", id, "", "")
	})
	return result(err, ResCreated, what)
}

// setNaming puts one moved naming value on s.
func setNaming(s *naming.Settings, k, v string) {
	switch k {
	case "rename_on_import":
		s.RenameOnImport = v == "true"
	case "colon":
		s.Colon = v
	case "illegal":
		s.Illegal = v
	case "file_pattern":
		s.FilePattern = v
	case "folder_pattern":
		s.FolderPattern = v
	case "show_folder_pattern":
		s.ShowFolderPattern = v
	case "season_folder_pattern":
		s.SeasonFolderPattern = v
	case "episode_pattern":
		s.EpisodePattern = v
	case "daily_pattern":
		s.DailyPattern = v
	case "anime_pattern":
		s.AnimePattern = v
	case "multi_episode_style":
		s.MultiEpisodeStyle = v
	}
}

func (r *run) naming(it Item) (string, string) {
	cur, err := naming.Read(r.ctx, r.m.Store.Read())
	if err != nil {
		return ResFailed, err.Error()
	}
	next := cur
	for k, v := range it.Naming {
		setNaming(&next, k, v)
	}
	if reflect.DeepEqual(next, cur) {
		return ResAlready, "naming is already set this way"
	}
	if err := next.Validate(); err != nil {
		return ResFailed, "these naming settings do not fit together: " + err.Error()
	}
	if r.dry {
		return ResChange, it.Detail + "; nothing on disk is renamed"
	}
	before, _ := json.Marshal(cur)
	after, _ := json.Marshal(it.Naming)
	err = naming.Save(r.ctx, r.m.Store, next, r.by, func(tx *sql.Tx) error {
		return r.markTx(tx, it.Key, "naming", 0, string(before), string(after))
	})
	return result(err, ResChanged, it.Detail+"; nothing on disk is renamed")
}

func (r *run) subtitles(it Item) (string, string) {
	sp := it.Subtitles
	if sp.SiteKind != "" {
		if r.id("SELECT id FROM subtitle_sites WHERE kind = ?", sp.SiteKind) != 0 {
			return ResAlready, "a subtitle site of this kind is here"
		}
		if r.dry {
			return ResCreate, "subtitle site " + sp.SiteKind
		}
		name := it.Name
		if r.id("SELECT id FROM subtitle_sites WHERE name = ?", name) != 0 {
			name = clip(name+" (moved)", 100)
		}
		on := !sp.NeedsAccount
		_, err := subtitles.AddSite(r.ctx, r.m.Store, nil, subtitles.SiteInput{Name: name, Kind: sp.SiteKind, Enabled: &on}, r.by, r.markWith(it.Key, "subtitle_site"))
		return result(err, ResCreated, "subtitle site "+sp.SiteKind)
	}
	cur := subtitles.ReadSettings(r.ctx, r.m.Store.Read())
	next := cur
	next.Languages = nil
	for _, w := range sp.Languages {
		next.Languages = append(next.Languages, subtitles.Want{Language: w.Language, HearingImpaired: w.HearingImpaired, Forced: w.Forced})
	}
	if reflect.DeepEqual(next.Languages, cur.Languages) {
		return ResAlready, "these are already the default languages"
	}
	if err := next.Validate(); err != nil {
		return ResFailed, "the languages were not taken: " + err.Error()
	}
	if r.dry {
		return ResChange, "default languages: " + it.Detail
	}
	before, _ := json.Marshal(cur.Languages)
	after, _ := json.Marshal(next.Languages)
	err := subtitles.SaveSettings(r.ctx, r.m.Store, next, r.by, func(tx *sql.Tx) error {
		return r.markTx(tx, it.Key, "subtitle_languages", 0, string(before), string(after))
	})
	return result(err, ResChanged, "default languages: "+it.Detail)
}

func (r *run) person(it Item) (string, string) {
	pp := it.Person
	if pp.Role == "owner" {
		r.people[it.Key] = r.owner
		return ResMatched, "is you, as you confirmed"
	}
	if pp.Choice == PersonSkip {
		return ResSkipped, "you left them out; their asks come over as yours"
	}
	// A person here with the same name is never merged silently: the owner
	// chooses merge or skip on the move screen.
	if id := r.id("SELECT id FROM users WHERE name = ? COLLATE NOCASE", it.Name); id != 0 {
		if pp.Choice != PersonMerge {
			return ResSkipped, "a person with the same name exists here: choose merge or skip on the move screen (until then their asks come over as yours)"
		}
		r.people[it.Key] = id
		return ResMatched, "merged with the person of this name here, as you chose; their asks are theirs"
	}
	email := pp.Email
	if email != "" && !alerts.CheckAddress(email) {
		r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupPeople, From: it.From, What: it.Name + ": email",
			Why: "not one plain email address, so the email is not moved"})
		email = ""
	}
	if r.dry {
		r.people[it.Key] = -1
		return ResCreate, "a turned-off " + pp.Role + " placeholder until you invite them"
	}
	perms, _ := json.Marshal(pp.Perms)
	u := domain.User{Name: it.Name, Role: pp.Role, PasswordHash: "", State: "disabled", Perms: string(perms)}
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		var err error
		if id, err = store.Insert(tx, "users", &u, r.by); err != nil {
			return err
		}
		var week any
		if pp.PerWeek != nil {
			week = *pp.PerWeek
		}
		if _, err = tx.Exec("INSERT INTO user_ask_settings (user_id, per_week, email) VALUES (?, ?, ?)", id, week, email); err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "user", id, "", "")
	})
	if err == nil {
		r.people[it.Key] = id
	}
	return result(err, ResCreated, "a turned-off "+pp.Role+" placeholder until you invite them")
}

func (r *run) personID(key string) int64 {
	if key == "" {
		return 0
	}
	if id, ok := r.people[key]; ok {
		return id
	}
	if id, ok := r.made(key); ok {
		return id
	}
	return 0
}

func (r *run) ask(it Item) (string, string) {
	a := it.Ask
	if a.Kind == "show" && !r.m.partOn("shows") || a.Kind == "movie" && !r.m.partOn("movies") {
		return ResSkipped, "its part is off"
	}
	tmdb := a.IDs["tmdb"]
	if r.id(`SELECT id FROM asks WHERE kind = ? AND state IN ('asked', 'approved') AND (ref = ? OR (tmdb_id != '' AND tmdb_id = ?))`, a.Kind, a.Ref, tmdb) != 0 {
		return ResAlready, "already on the Wishlist"
	}
	var titleID int64
	if a.Kind == "movie" {
		titleID, _ = library.TitleByTMDB(r.ctx, r.m.Store.Read(), tmdb)
	} else {
		titleID, _ = library.ShowByIDs(r.ctx, r.m.Store.Read(), a.IDs)
	}
	by := r.personID(a.By)
	note := a.Note
	if by == 0 {
		by = r.owner
		if a.ByName != "" {
			note = strings.TrimSpace(note + "; asked by " + a.ByName + " in " + a.App)
		}
	}
	state := "asked"
	if a.Approved && titleID != 0 {
		state = "approved"
	} else if a.Approved {
		note = strings.TrimSpace(note + "; approved in " + a.App + ": approve it again here to add it")
	}
	note = strings.Trim(note, "; ")
	what := map[string]string{"asked": "waiting", "approved": "approved"}[state]
	if r.dry {
		if by < 0 {
			return ResCreate, what + ", by the new placeholder"
		}
		return ResCreate, what
	}
	if by <= 0 {
		return ResFailed, "its asker was not moved"
	}
	asked := r.m.now()
	if t, err := time.Parse(time.RFC3339, a.AskedAt); err == nil && t.Before(asked) {
		asked = t.UTC()
	}
	row := domain.Ask{Kind: a.Kind, TMDBID: tmdb, Name: it.Name, State: state, AskedBy: by, AskedAt: asked, Note: clip(note, 300),
		Ref: a.Ref, SourceIDs: a.IDs, Seasons: append([]int{}, a.Seasons...), Albums: []domain.AskAlbum{}}
	if a.Kind == "movie" {
		row.Seasons = []int{}
	}
	if titleID != 0 {
		row.TitleID = &titleID
	}
	if state == "approved" {
		d := r.personID(a.Decider)
		if d <= 0 {
			d = r.owner
		}
		now := r.m.now()
		row.DecidedBy, row.DecidedAt = &d, &now
	}
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		var err error
		if id, err = store.Insert(tx, "asks", &row, r.by); err != nil {
			return err
		}
		// A moved ask never counts against the person's weekly limit.
		if _, err = tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)", id, by, store.FormatTime(asked)); err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "ask", id, "", "")
	})
	return result(err, ResCreated, what)
}

func (r *run) alert(it Item) (string, string) {
	a := it.Alert
	if r.id("SELECT id FROM alerts WHERE name = ? COLLATE NOCASE", it.Name) != 0 {
		return ResAlready, "an Alert with this name is here"
	}
	what := a.Kind + ", off until you type " + it.Needs
	if r.dry {
		return ResCreate, what
	}
	row := domain.Alert{Name: it.Name, Kind: a.Kind, Enabled: false, Moments: append([]string{}, alerts.Moments...), SMTPHost: a.SMTPHost,
		SMTPPort: 587, SMTPSecurity: "starttls", SMTPFrom: a.SMTPFrom, SMTPUser: a.SMTPUser, StaffTo: a.StaffTo, WebhookAuth: "sign"}
	if a.SMTPPort > 0 {
		row.SMTPPort = a.SMTPPort
	}
	if a.SMTPSecurity != "" {
		row.SMTPSecurity = a.SMTPSecurity
	}
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		id, err := store.Insert(tx, "alerts", &row, r.by)
		if err != nil {
			return err
		}
		return r.markTx(tx, it.Key, "alert", id, "", "")
	})
	return result(err, ResCreated, what)
}

// Move is one move's row.
type Move struct {
	ID         int64      `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	OwnerID    int64      `json:"owner_id"`
	Snapshots  []int64    `json:"snapshots"`
	Choices    Choices    `json:"choices"`
	State      string     `json:"state"`
	JobID      int64      `json:"job_id,omitempty"`
	Report     *Report    `json:"report,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	UndoneAt   *time.Time `json:"undone_at,omitempty"`
	// UndoState follows an undo Job: queued, running or failed ("" when
	// there is none or it finished, which makes the move undone).
	UndoState string `json:"undo_state,omitempty"`
	UndoJobID int64  `json:"undo_job_id,omitempty"`
}

// busySQL counts moves and undos still running. A move or undo whose Job is
// gone (a crash) does not count, so it never blocks forever; Recover fails
// it at start.
const busySQL = `SELECT count(*) FROM moves m LEFT JOIN jobs a ON a.id = m.job_id LEFT JOIN jobs u ON u.id = m.undo_job_id
	WHERE (m.state IN ('queued', 'running') AND (m.job_id IS NULL OR a.state IN ('queued', 'running')))
	OR (m.undo_state IN ('queued', 'running') AND (m.undo_job_id IS NULL OR u.state IN ('queued', 'running')))`

func busy(tx *sql.Tx) error {
	var n int
	if err := tx.QueryRow(busySQL).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrMoveBusy
	}
	return nil
}

// Start keeps an approved move and queues its Job. planHash must be the
// PlanHash of the plan the dry run showed for these snapshots and choices.
func (m *Mover) Start(ctx context.Context, snaps []int64, ch Choices, planHash string, by domain.Actor) (Move, error) {
	p, err := LoadPlan(ctx, m.Store, snaps, ch)
	if err != nil {
		return Move{}, err
	}
	if planHash == "" || planHash != PlanHash(p) {
		return Move{}, ErrPlanChanged
	}
	if m.Queue == nil {
		return Move{}, errors.New("no job queue")
	}
	if ch.PathRules == nil {
		ch.PathRules = []PathRule{}
	}
	if ch.Skip == nil {
		ch.Skip = []string{}
	}
	sj, _ := json.Marshal(snaps)
	cj, _ := json.Marshal(ch)
	mv := Move{CreatedAt: m.now().Truncate(time.Second), OwnerID: by.ID, Snapshots: snaps, Choices: ch, State: "queued"}
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := busy(tx); err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, snapshots, choices, state) VALUES (?, ?, ?, ?, 'queued')",
			mv.CreatedAt.Format(time.RFC3339), by.ID, string(sj), string(cj))
		if err != nil {
			return err
		}
		mv.ID, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return mv, err
	}
	jid, _, err := m.Queue.Enqueue(ctx, KindMove, &domain.Subject{Type: "move", ID: mv.ID}, nil, KindMove+":"+strconv.FormatInt(mv.ID, 10), by)
	if err != nil {
		_ = m.setState(ctx, mv.ID, "failed", nil)
		return mv, err
	}
	mv.JobID = jid
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET job_id = ? WHERE id = ?", jid, mv.ID)
		return err
	})
	return mv, err
}

func (m *Mover) setState(ctx context.Context, id int64, state string, rep *Report) error {
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		if rep == nil {
			_, err := tx.Exec("UPDATE moves SET state = ? WHERE id = ?", state, id)
			return err
		}
		b, _ := json.Marshal(rep)
		if state == "undone" {
			_, err := tx.Exec("UPDATE moves SET state = 'undone', undo_state = '', report = ?, undone_at = ? WHERE id = ?", string(b), store.FormatTime(m.now()), id)
			return err
		}
		_, err := tx.Exec("UPDATE moves SET state = ?, report = ?, finished_at = ? WHERE id = ?", state, string(b), store.FormatTime(m.now()), id)
		return err
	})
}

func (m *Mover) setUndoState(ctx context.Context, id int64, state string) error {
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET undo_state = ? WHERE id = ?", state, id)
		return err
	})
}

// stoppedNote is the report of a move a stop cut off with no Job left.
const stoppedNote = "HolyArrs stopped during this move, before it finished. What it made is marked: move again to finish it (nothing is added twice), or undo it."

// Recover runs at start, before the Job queue: a move or undo whose Job is
// gone (not queued or running) failed with a stop, so it is marked failed
// instead of blocking every later move. A move whose Job is still there is
// resumed by the queue (a move is safe to repeat).
func (m *Mover) Recover(ctx context.Context) error {
	b, _ := json.Marshal(&Report{Lines: []Line{}, Counts: map[string]int{}, NotMoved: []NotMoved{}, NeedsInput: []string{}, Next: []string{stoppedNote}})
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE moves SET state = 'failed', report = CASE WHEN report IN ('', '{}') THEN ? ELSE report END, finished_at = ?
			WHERE state IN ('queued', 'running') AND (job_id IS NULL OR NOT EXISTS (SELECT 1 FROM jobs j WHERE j.id = moves.job_id AND j.state IN ('queued', 'running')))`,
			string(b), store.FormatTime(m.now()))
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE moves SET undo_state = 'failed' WHERE undo_state IN ('queued', 'running')
			AND (undo_job_id IS NULL OR NOT EXISTS (SELECT 1 FROM jobs j WHERE j.id = moves.undo_job_id AND j.state IN ('queued', 'running')))`)
		return err
	})
}

func (m *Mover) handle(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "move" {
		return errors.New("a move Job needs a move subject")
	}
	mv, err := GetMove(ctx, m.Store, r.Job.Subject.ID)
	if err != nil {
		return err
	}
	if mv.State != "queued" && mv.State != "running" {
		return nil
	}
	if err := m.setState(ctx, mv.ID, "running", nil); err != nil {
		return err
	}
	p, err := LoadPlan(ctx, m.Store, mv.Snapshots, mv.Choices)
	if err != nil {
		_ = m.setState(context.WithoutCancel(ctx), mv.ID, "failed", &Report{Move: mv.ID, Lines: []Line{}, Counts: map[string]int{},
			Next: []string{"The move could not start: " + err.Error()}})
		return err
	}
	by := r.Job.CreatedBy
	rep := m.Run(ctx, mv.ID, p, false, by, func(done, total int) {
		_ = r.Progress(ctx, float64(done)/float64(max(total, 1)), fmt.Sprintf("%d of %d moved", done, total))
	})
	state := "done"
	if ctx.Err() != nil {
		state = "failed"
	}
	if err := m.setState(context.WithoutCancel(ctx), mv.ID, state, rep); err != nil {
		return err
	}
	r.SetResult(map[string]any{"move": mv.ID, "counts": rep.Counts})
	return ctx.Err()
}

// GetMove reads one move.
func GetMove(ctx context.Context, st *store.Store, id int64) (Move, error) {
	ms, err := moves(ctx, st, "WHERE id = ?", id)
	if err != nil {
		return Move{}, err
	}
	if len(ms) == 0 {
		return Move{}, ErrNoMove
	}
	return ms[0], nil
}

// ListMoves lists the moves, newest first, without their reports.
func ListMoves(ctx context.Context, st *store.Store) ([]Move, error) {
	ms, err := moves(ctx, st, "ORDER BY id DESC LIMIT 100")
	for i := range ms {
		if ms[i].Report != nil {
			ms[i].Report = &Report{Counts: ms[i].Report.Counts, Lines: []Line{}, NotMoved: []NotMoved{}, NeedsInput: []string{}, Next: []string{}}
		}
	}
	return ms, err
}

func moves(ctx context.Context, st *store.Store, where string, args ...any) ([]Move, error) {
	rows, err := st.Read().QueryContext(ctx, `SELECT id, created_at, owner_id, snapshots, choices, state, COALESCE(job_id, 0), report,
		COALESCE(finished_at, ''), COALESCE(undone_at, ''), undo_state, COALESCE(undo_job_id, 0) FROM moves `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Move{}
	for rows.Next() {
		var mv Move
		var created, snaps, ch, rep, fin, und string
		if err := rows.Scan(&mv.ID, &created, &mv.OwnerID, &snaps, &ch, &mv.State, &mv.JobID, &rep, &fin, &und, &mv.UndoState, &mv.UndoJobID); err != nil {
			return nil, err
		}
		mv.CreatedAt, _ = time.Parse(time.RFC3339, created)
		_ = json.Unmarshal([]byte(snaps), &mv.Snapshots)
		_ = json.Unmarshal([]byte(ch), &mv.Choices)
		if rep != "{}" && rep != "" {
			mv.Report = &Report{}
			_ = json.Unmarshal([]byte(rep), mv.Report)
		}
		if t, err := time.Parse(time.RFC3339Nano, fin); err == nil && fin != "" {
			mv.FinishedAt = &t
		}
		if t, err := time.Parse(time.RFC3339Nano, und); err == nil && und != "" {
			mv.UndoneAt = &t
		}
		out = append(out, mv)
	}
	return out, rows.Err()
}

// StartUndo queues the undo of a finished move as a supervised Job (with
// its own progress); it counts in the busy check like a move.
func (m *Mover) StartUndo(ctx context.Context, id int64, by domain.Actor) (Move, error) {
	mv, err := GetMove(ctx, m.Store, id)
	if err != nil {
		return mv, err
	}
	switch {
	case mv.State == "undone":
		return mv, ErrUndone
	case mv.State != "done" && mv.State != "failed":
		return mv, ErrMoveBusy
	}
	if m.Queue == nil {
		return mv, errors.New("no job queue")
	}
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := busy(tx); err != nil {
			return err
		}
		res, err := tx.Exec("UPDATE moves SET undo_state = 'queued', undo_job_id = NULL WHERE id = ? AND state IN ('done', 'failed')", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrMoveBusy
		}
		return nil
	})
	if err != nil {
		return mv, err
	}
	jid, _, err := m.Queue.Enqueue(ctx, KindUndo, &domain.Subject{Type: "move", ID: id}, nil, KindUndo+":"+strconv.FormatInt(id, 10), by)
	if err != nil {
		_ = m.setUndoState(ctx, id, "failed")
		return mv, err
	}
	err = m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET undo_job_id = ? WHERE id = ?", jid, id)
		return err
	})
	mv.UndoState, mv.UndoJobID = "queued", jid
	return mv, err
}

func (m *Mover) handleUndo(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "move" {
		return errors.New("an undo Job needs a move subject")
	}
	mv, err := GetMove(ctx, m.Store, r.Job.Subject.ID)
	if err != nil {
		return err
	}
	if mv.State == "undone" || mv.UndoState != "queued" && mv.UndoState != "running" {
		return nil
	}
	if err := m.setUndoState(ctx, mv.ID, "running"); err != nil {
		return err
	}
	rep, err := m.undo(ctx, mv.ID, r.Job.CreatedBy, func(done, total int) {
		_ = r.Progress(ctx, float64(done)/float64(max(total, 1)), fmt.Sprintf("%d of %d undone", done, total))
	})
	if err != nil {
		_ = m.setUndoState(context.WithoutCancel(ctx), mv.ID, "failed")
		return err
	}
	r.SetResult(map[string]any{"move": mv.ID, "counts": rep.Counts})
	return nil
}

// madeRow is one marked row of a move.
type madeRow struct {
	key, what, before, after, fp string
	row                          int64
}

// undo removes what a move made, newest first, and never a file: titles
// leave the Library with their files where they are. A row is removed only
// while its fingerprint holds (SQLite reuses ids), and a setting is put
// back only while it still has the value the move set. A thing the owner
// has since set up or changed stays, and the report says so.
func (m *Mover) undo(ctx context.Context, id int64, by domain.Actor, progress func(done, total int)) (*Report, error) {
	mv, err := GetMove(ctx, m.Store, id)
	if err != nil {
		return nil, err
	}
	switch mv.State {
	case "queued", "running":
		return nil, ErrMoveBusy
	case "undone":
		return nil, ErrUndone
	}
	rows, err := m.Store.Read().QueryContext(ctx, "SELECT key, what, row_id, before, after, fingerprint FROM move_made WHERE move_id = ? ORDER BY rowid DESC", id)
	if err != nil {
		return nil, err
	}
	var made []madeRow
	for rows.Next() {
		var x madeRow
		if err := rows.Scan(&x.key, &x.what, &x.row, &x.before, &x.after, &x.fp); err != nil {
			rows.Close()
			return nil, err
		}
		made = append(made, x)
	}
	rows.Close()
	rep := &Report{Move: id, Undo: true, At: m.now(), Lines: []Line{}, Counts: map[string]int{}, NotMoved: []NotMoved{}, NeedsInput: []string{}, Next: []string{}}
	for i, x := range made {
		if ctx.Err() != nil {
			return rep, ctx.Err()
		}
		res, detail := m.undoOne(ctx, x, by)
		rep.Lines = append(rep.Lines, Line{Group: x.what, Key: x.key, Name: x.key, Result: res, Detail: detail})
		rep.Counts[res]++
		if progress != nil && ((i+1)%25 == 0 || i+1 == len(made)) {
			progress(i+1, len(made))
		}
	}
	rep.Next = []string{"Nothing in the old apps was touched by the move or by this undo, and no media file was moved or removed."}
	if rep.Counts[ResKept] > 0 {
		rep.Next = append(rep.Next, "Some things stay because you set them up or changed them after the move, or they are no longer the rows the move made; remove them by hand if you want.")
	}
	if err := m.setState(context.WithoutCancel(ctx), id, "undone", rep); err != nil {
		return rep, err
	}
	return rep, nil
}

// Undo runs an undo now (the undo Job's work); the routes use StartUndo.
func (m *Mover) Undo(ctx context.Context, id int64, by domain.Actor) (*Report, error) {
	return m.undo(ctx, id, by, nil)
}

const (
	keptReplaced = "this is no longer the row the move made (it was removed and its place reused, or renamed), so it stays"
	keptUnknown  = "it cannot be checked to be the row the move made (marked before this check existed), so it stays; remove it by hand if you want"
	keptChanged  = "you changed it since the move, so it stays as you left it"
)

// undoOne undoes one marked row.
func (m *Mover) undoOne(ctx context.Context, x madeRow, by domain.Actor) (string, string) {
	exec := func(q string, args ...any) (int64, error) {
		var n int64
		err := m.Store.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec(q, args...)
			if err != nil {
				return err
			}
			n, _ = res.RowsAffected()
			return nil
		})
		return n, err
	}
	// Rows are checked against their fingerprint first; the delete itself
	// requires it again, so a row replaced in between is never removed.
	if n, ok := natural[x.what]; ok {
		if x.fp == "" {
			return ResKept, keptUnknown
		}
		switch fingerprint(ctx, m.Store.Read(), x.what, x.row) {
		case "":
			return ResKept, "already gone"
		case x.fp:
		default:
			return ResKept, keptReplaced
		}
		del := func(extra, why string) (string, string) {
			c, err := exec("DELETE FROM "+n[0]+" WHERE id = ? AND created_at || char(31) || "+n[1]+" = ?"+extra, x.row, x.fp)
			switch {
			case err != nil:
				return ResFailed, err.Error()
			case c == 0:
				return ResKept, why
			}
			return ResRemoved, ""
		}
		switch x.what {
		case "ask":
			return del(" AND state IN ('asked', 'approved')", "an ask that was decided since stays")
		case "user":
			return del(" AND state = 'disabled' AND password_hash = '' AND role != 'owner' AND NOT EXISTS (SELECT 1 FROM sessions WHERE user_id = users.id)",
				"this person was turned on since, so they stay")
		case "alert":
			return del(" AND url_secret_id IS NULL AND password_secret_id IS NULL", "you typed its secret since, so it stays; remove it under Settings > Alerts")
		case "subtitle_site":
			return del(" AND secret_ref IS NULL", "you gave it an account since, so it stays")
		case "source":
			return del(" AND secret_ref IS NULL", "you typed its key since, so it stays; remove it under Settings > Sources")
		case "downloader":
			return del(" AND secret_ref IS NULL AND NOT EXISTS (SELECT 1 FROM downloads WHERE downloader_id = downloaders.id)",
				"it is set up or has downloads now, so it stays; remove it under Settings > Downloaders")
		case "never_add":
			return del("", "already gone")
		case "title":
			if _, e := m.Library.Remove(ctx, x.row, library.RemoveChoice{}, true, by); e != nil && !errors.Is(e, store.ErrNotFound) && !errors.Is(e, library.ErrAlreadyRemoved) {
				return ResFailed, e.Error()
			}
			return ResRemoved, "left the Library; its files stay where they are"
		case "folder":
			e := library.DeleteFolder(ctx, m.Store, x.row)
			switch {
			case e == nil:
				return ResRemoved, ""
			case errors.Is(e, library.ErrFolderInUse):
				return ResKept, "the titles that were in it stay in History as removed, so the folder stays"
			case errors.Is(e, store.ErrNotFound):
				return ResKept, "already gone"
			}
			return ResFailed, e.Error()
		}
	}
	switch x.what {
	case "naming":
		var moved map[string]string
		var old naming.Settings
		if x.after == "" || json.Unmarshal([]byte(x.after), &moved) != nil || json.Unmarshal([]byte(x.before), &old) != nil {
			return ResKept, keptUnknown
		}
		cur, err := naming.Read(ctx, m.Store.Read())
		if err != nil {
			return ResFailed, err.Error()
		}
		same := cur
		for k, v := range moved {
			setNaming(&same, k, v)
		}
		if !reflect.DeepEqual(same, cur) {
			return ResKept, keptChanged
		}
		if err := naming.Save(ctx, m.Store, old, by); err != nil {
			return ResFailed, err.Error()
		}
		return ResRemoved, "naming is back as it was"
	case "subtitle_languages":
		var old []subtitles.Want
		if x.after == "" || json.Unmarshal([]byte(x.before), &old) != nil {
			return ResKept, keptUnknown
		}
		cur := subtitles.ReadSettings(ctx, m.Store.Read())
		now, _ := json.Marshal(cur.Languages)
		if string(now) != x.after {
			return ResKept, keptChanged
		}
		cur.Languages = old
		if err := subtitles.SaveSettings(ctx, m.Store, cur, by); err != nil {
			return ResFailed, err.Error()
		}
		return ResRemoved, "the default languages are back as they were"
	}
	return ResKept, "nothing to undo for it"
}
