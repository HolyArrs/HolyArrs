package migrate

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
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
	// Choose says this person needs (or had) the owner's Same person or
	// Leave out choice, and why: ChooseSameName, ChooseNumericID or
	// ChooseMergeGone. It stays after a choice is made, so the screen keeps
	// the row and the choice can be changed.
	Choose string `json:"choose,omitempty"`
	// open says the choice is still missing (the person stays behind).
	open bool
}

// Why a person needs the owner's choice (Line.Choose).
const (
	ChooseSameName  = "same_name"  // someone here has the same name
	ChooseNumericID = "numeric_id" // only a numeric id, which cannot prove the same person
	ChooseMergeGone = "merge_gone" // the person picked for Same person is no longer here
)

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
	Store   *database.Store
	Library *library.Service
	Queue   *jobs.Queue
	// PartOn says whether a part is on. The part keys are movies, shows,
	// music, subtitles and wishlist (the requests part keeps its old key);
	// nil counts every part on.
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
func LoadPlan(ctx context.Context, st *database.Store, ids []int64, ch Choices) (*Plan, error) {
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
// name, or else to the only Library folder here of the same kind (an old
// Sonarr "/data/TV Shows" goes to the one shows folder).
func SuggestRules(ctx context.Context, st *database.Store, p *Plan) []PathRule {
	var here []string
	kindOf := map[string]string{}
	rows, err := st.Read().QueryContext(ctx, "SELECT path, kind FROM library_folders ORDER BY id")
	if err == nil {
		for rows.Next() {
			var s, k string
			if rows.Scan(&s, &k) == nil {
				here = append(here, s)
				kindOf[s] = k
			}
		}
		rows.Close()
	}
	oldKind := map[string]string{}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if it.Folder != nil {
				oldKind[it.Folder.OldPath] = it.Folder.Kind
			}
		}
	}
	out := []PathRule{}
	for _, old := range p.Paths {
		if isDir(filepath.FromSlash(old)) {
			continue
		}
		base := strings.ToLower(old[strings.LastIndexByte(old, '/')+1:])
		found := ""
		for _, h := range here {
			if strings.ToLower(filepath.Base(h)) == base {
				found = h
				break
			}
		}
		if found == "" {
			var same []string
			for _, h := range here {
				if k := oldKind[old]; k != "" && kindOf[h] == k {
					same = append(same, h)
				}
			}
			if len(same) == 1 {
				found = same[0]
			}
		}
		if found != "" {
			out = append(out, PathRule{From: old, To: filepath.ToSlash(found)})
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
	case it.OutsideList != nil:
		kind = "movie" // Outside lists add movies
	case it.Protocol != nil:
		return it.Protocol.Part
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
	moveAt time.Time
	dry    bool
	by     domain.Actor
	rep    *Report
	people map[string]int64
	// personStable records whether a planned person has a provider-scoped
	// identity. It is populated before any groups run so request ownership
	// stays safe even when the People group itself is skipped.
	personStable map[string]bool
	// personMarked says an older mark may resolve this planned person. A
	// skipped group and explicit skip/merge/owner choices must be resolved by
	// this run (or fall back to the current owner), never by an older mark.
	personMarked map[string]bool
	owner        int64
	// choose is set by one() for the item it just ran (Line.Choose).
	choose string
}

// referenceTime returns the persisted start of this move. Source timestamps
// that could not be preserved use this stable value, so a retry after a stop
// cannot silently change request history to a later wall-clock time.
func (r *run) referenceTime() (time.Time, error) {
	if !r.moveAt.IsZero() {
		return r.moveAt, nil
	}
	var created string
	if err := r.m.Store.Read().QueryRowContext(r.ctx, "SELECT created_at FROM moves WHERE id = ?", r.move).Scan(&created); err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return time.Time{}, err
	}
	r.moveAt = t.UTC()
	return r.moveAt, nil
}

func (r *run) sourceOrMoveTime(source string) (time.Time, error) {
	if t, ok := sourceTime(source); ok {
		return t.UTC(), nil
	}
	return r.referenceTime()
}

// Run dry-runs (dry) or applies a Plan. progress, when set, hears how far
// it is.
func (m *Mover) Run(ctx context.Context, moveID int64, p *Plan, dry bool, by domain.Actor, progress func(done, total int)) *Report {
	r := &run{m: m, ctx: ctx, move: moveID, dry: dry, by: by, people: map[string]int64{}, personStable: map[string]bool{}, personMarked: map[string]bool{},
		rep: &Report{Move: moveID, Dry: dry, Apps: p.Apps, At: m.now(), Lines: []Line{}, Counts: map[string]int{},
			NotMoved: append([]NotMoved{}, p.NotMoved...), NeedsInput: append([]string{}, p.NeedsInput...), Notes: p.Notes}}
	for _, g := range p.Groups {
		for _, it := range g.Items {
			if it.Person != nil {
				r.personStable[it.Key] = it.Person.StableIdentity
				r.personMarked[it.Key] = !g.Skipped && it.Person.Choice != PersonSkip && it.Person.Choice != PersonMerge && it.Person.Role != "owner"
			}
		}
	}
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
				r.line(g.Name, it, ResFailed, "the import was stopped before this", "")
				continue
			}
			var res, detail string
			r.choose = ""
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
			if r.choose != "" {
				l := &r.rep.Lines[len(r.rep.Lines)-1]
				l.Choose = r.choose
				l.open = res == ResSkipped && it.Person != nil && it.Person.Choice != PersonSkip
			}
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
		out = append(out, "Some folders were not found on this server: add path rules and import again (nothing is added twice).")
	}
	if len(rep.NeedsInput) > 0 {
		out = append(out, "Type the keys and passwords listed under \"needs your input\": Sources, Downloaders, Alerts and subtitle sites stay paused until then.")
	}
	links, people, same, outside, trakt := false, false, false, false, false
	for _, l := range rep.Lines {
		made := l.Result == ResCreated || l.Result == ResCreate
		switch {
		case l.Group == GroupLists && made:
			outside = true
			trakt = trakt || strings.HasPrefix(l.Key, "outside_list:"+requests.ListTrakt+":")
		case (l.Group == GroupSources || l.Group == GroupDownloaders) && made:
			links = true
		case l.Group == GroupPeople && made:
			people = true
		case l.Group == GroupPeople && l.Choose != "" && l.open:
			same = true
		}
	}
	if links {
		out = append(out, "Moved Sources and Downloaders are always added paused and cannot reach your home network. Check each one under Settings > Sources and Settings > Downloaders, turn on \"May reach the local network\" only for one that lives on your network, then resume it.")
	}
	if outside {
		out = append(out, "Outside lists come over paused; turn each on under Settings > Outside lists after checking its folder and Pick rules.")
	}
	if trakt {
		out = append(out, "Add your Trakt key under Settings > Outside lists before you turn a Trakt list on; the list must be public.")
	}
	if people {
		out = append(out, "People came over turned off, as members (admin or owner only where you confirmed it): invite them from Settings > Users and roles.")
	}
	if same {
		out = append(out, "Some people need a choice: pick Same person or Leave out for each, then try and import again.")
	}
	if rep.Counts[ResFailed] > 0 {
		out = append(out, "Some items failed; their reason is on their line. Fix it and import again: the old apps' keys are kept until an import finishes with nothing failed.")
	}
	if len(rep.NotMoved) > 0 {
		out = append(out, "Look over the not-imported list: those settings have no HolyArrs match and stay in the old apps.")
	}
	out = append(out, "Keep the old apps until HolyArrs runs the way you want; this import changed nothing in them.")
	return out
}

// made says whether a live (not undone) move already made key, and its row.
func (r *run) made(key string) (int64, bool) {
	var id int64
	var what, marked string
	err := r.m.Store.Read().QueryRowContext(r.ctx, `SELECT m.row_id, m.what, m.fingerprint FROM move_made m JOIN moves v ON v.id = m.move_id
		WHERE m.key = ? AND v.state != 'undone' ORDER BY m.rowid DESC LIMIT 1`, key).Scan(&id, &what, &marked)
	if err != nil {
		return 0, false
	}
	return r.validateMade(id, what, marked)
}

func (r *run) madeThisMove(key string) (int64, bool) {
	var id int64
	var what, marked string
	err := r.m.Store.Read().QueryRowContext(r.ctx, `SELECT row_id, what, fingerprint FROM move_made
		WHERE move_id = ? AND key = ? ORDER BY rowid DESC LIMIT 1`, r.move, key).Scan(&id, &what, &marked)
	if err != nil {
		return 0, false
	}
	return r.validateMade(id, what, marked)
}

func (r *run) validateMade(id int64, what, marked string) (int64, bool) {
	fingerprintKind := markFingerprintKind(what)
	if _, rowIdentity := natural[fingerprintKind]; !rowIdentity {
		return id, true
	}
	// Natural-row marks written before fingerprints existed cannot prove that
	// a recycled SQLite id still belongs to the imported person or other row.
	// Retrying conservatively is safer than attaching ownership to a stranger.
	if marked == "" {
		return 0, false
	}
	current := fingerprint(r.ctx, r.m.Store.Read(), fingerprintKind, id)
	if current == "" {
		return 0, false
	}
	if what == "title" || what == "music_policy" {
		var state string
		if r.m.Store.Read().QueryRowContext(r.ctx, "SELECT state FROM titles WHERE id = ?", id).Scan(&state) != nil || state == "removed" {
			return 0, false
		}
		if current != marked {
			return 0, false
		}
	} else if !sameMarkedCreation(marked, current) {
		return 0, false
	}
	return id, true
}

func (r *run) madeItem(it Item) (int64, bool) {
	if id, ok := r.made(it.Key); ok {
		return id, true
	}
	// Numeric person aliases predate stable provider identities. They cannot
	// prove that a reused source id still names the same person, so request
	// ownership must never be recovered through them.
	if it.Person != nil {
		return 0, false
	}
	for _, key := range it.LegacyKeys {
		if id, ok := r.made(key); ok {
			return id, true
		}
	}
	return 0, false
}

func (r *run) hasActiveMark(key string) bool {
	var one int
	err := r.m.Store.Read().QueryRowContext(r.ctx, `SELECT 1 FROM move_made m JOIN moves v ON v.id=m.move_id
		WHERE m.key=? AND v.state!='undone' LIMIT 1`, key).Scan(&one)
	return err == nil
}

// natural names, per kind of marked row, its table and the column that
// with created_at fingerprints it: SQLite gives a deleted row's id to the
// next row, so undo deletes a row only while its fingerprint still holds.
var natural = map[string][2]string{
	"ask": {"asks", "name"}, "user": {"users", "name"}, "alert": {"alerts", "name"}, "subtitle_site": {"subtitle_sites", "name"},
	"source": {"sources", "name"}, "downloader": {"downloaders", "name"}, "never_add": {"never_add", "name"},
	"title": {"titles", "kind"}, "folder": {"library_folders", "path"}, "outside_list": {"outside_lists", "name"},
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// fingerprint reads a marked row's created_at and natural key ("" when the
// row is gone or its kind has none).
func fingerprint(ctx context.Context, q rowQuerier, what string, id int64) string {
	fp, _ := readFingerprint(ctx, q, what, id)
	return fp
}

func readFingerprint(ctx context.Context, q rowQuerier, what string, id int64) (string, error) {
	n, ok := natural[what]
	if !ok {
		return "", nil
	}
	var fp string
	err := q.QueryRowContext(ctx, "SELECT created_at || char(31) || "+n[1]+" FROM "+n[0]+" WHERE id = ?", id).Scan(&fp)
	return fp, err
}

func markFingerprintKind(what string) string {
	if what == "music_policy" {
		return "title"
	}
	if what == "ask_claim" {
		return "ask"
	}
	return what
}

func sameMarkedCreation(marked, current string) bool {
	mi := strings.IndexByte(marked, 31)
	ci := strings.IndexByte(current, 31)
	return mi > 0 && ci > 0 && marked[:mi] == current[:ci]
}

// markTx records, in the transaction that made the row, that this move
// made it, with its fingerprint; before and after are a changed setting's
// old and new values.
func (r *run) markTx(tx *sql.Tx, key, what string, id int64, before, after string) error {
	fp := ""
	fingerprintKind := markFingerprintKind(what)
	if _, rowKind := natural[fingerprintKind]; id != 0 && rowKind {
		if fp = fingerprint(r.ctx, tx, fingerprintKind, id); fp == "" {
			return errors.New("the new row was not found to mark")
		}
	}
	_, err := tx.Exec(`INSERT OR REPLACE INTO move_made (move_id, key, what, row_id, before, after, fingerprint, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.move, key, what, id, before, after, fp, database.FormatTime(r.m.now()))
	return err
}

// markWith is markTx as an after-insert step of another package's save.
func (r *run) markWith(key, what string) func(tx *sql.Tx, id int64) error {
	return func(tx *sql.Tx, id int64) error { return r.markTx(tx, key, what, id, "", "") }
}

func (r *run) one(group string, it Item) (string, string) {
	// Ask items deliberately bypass marks from earlier moves so a later move
	// can keep its dependency claim. A repeat of this same move must keep its
	// original ask/relationship mark intact for undo.
	if it.Ask != nil {
		if exists, current := r.askMarkThisMove(it.Key); exists {
			if current {
				return ResAlready, "imported before"
			}
			return ResFailed, "the earlier request mark no longer proves the same request owner and relationship"
		}
	}
	if it.Person != nil {
		if !it.Person.StableIdentity {
			if id, ok := r.madeThisMove(it.Key); ok {
				r.people[it.Key] = id
				return ResAlready, "imported before in this import"
			}
			if r.hasActiveMark(it.Key) && it.Person.Role != "owner" {
				r.choose = ChooseNumericID
				if it.Person.Choice != PersonMerge && it.Person.Choice != PersonSkip {
					r.people[it.Key] = -2
					return ResSkipped, "HolyArrs can't tell if this is the same person. Choose Same person or Leave out."
				}
			}
		}
		for _, legacy := range it.LegacyKeys {
			if r.hasActiveMark(legacy) && it.Person.Role != "owner" {
				r.choose = ChooseNumericID
				if it.Person.Choice != PersonMerge && it.Person.Choice != PersonSkip {
					r.people[it.Key] = -2
					return ResSkipped, "HolyArrs can't tell if this is the same person. Choose Same person or Leave out."
				}
			}
		}
	}
	if id, ok := r.madeItem(it); ok && group != GroupRules && it.Ask == nil &&
		(it.Person == nil || it.Person.StableIdentity && it.Person.Choice != PersonSkip && it.Person.Choice != PersonMerge && it.Person.Role != "owner") {
		if it.Person != nil {
			r.people[it.Key] = id
		}
		// A title moved before still takes the quality the old app recorded
		// for each file, so moving again after reading the old app updates it.
		if it.Title != nil && it.Title.Music == nil && len(it.Title.Qualities) > 0 && !r.dry {
			if n, err := r.m.Library.SetRecordedQualities(r.ctx, id, it.Title.Qualities, r.by); err == nil && n > 0 {
				return ResAlready, "imported before; the quality of " + strconv.Itoa(n) + " files was updated"
			}
		}
		return ResAlready, "imported before"
	}
	switch {
	case it.Folder != nil:
		return r.folder(it)
	case it.Rules != nil:
		return r.rules(it)
	case it.Protocol != nil:
		return r.protocol(it)
	case it.Title != nil:
		return r.title(it)
	case it.NeverAdd != nil:
		return r.neverAdd(it)
	case it.OutsideList != nil:
		return r.outsideList(it)
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
	case it.AskSettings != nil:
		return r.askSettings(it)
	case it.Ask != nil:
		return r.ask(it)
	case it.Alert != nil:
		return r.alert(it)
	}
	return ResFailed, "nothing to do for it"
}
