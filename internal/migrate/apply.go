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
	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/lists"
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
	links, people, same, outside, trakt := false, false, false, false, false
	for _, l := range rep.Lines {
		made := l.Result == ResCreated || l.Result == ResCreate
		switch {
		case l.Group == GroupLists && made:
			outside = true
			trakt = trakt || strings.HasPrefix(l.Key, "outside_list:"+lists.KindTrakt+":")
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
	if outside {
		out = append(out, "Outside lists come over paused; turn each on under Settings > Outside lists after checking its folder and Pick rules.")
	}
	if trakt {
		out = append(out, "Add your Trakt key under Settings > Outside lists before you turn a Trakt list on; the list must be public.")
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
		r.move, key, what, id, before, after, fp, store.FormatTime(r.m.now()))
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
				return ResAlready, "moved before"
			}
			return ResFailed, "the earlier request mark no longer proves the same request owner and relationship"
		}
	}
	if it.Person != nil {
		if !it.Person.StableIdentity {
			if id, ok := r.madeThisMove(it.Key); ok {
				r.people[it.Key] = id
				return ResAlready, "moved before in this move"
			}
			if r.hasActiveMark(it.Key) && it.Person.Role != "owner" && it.Person.Choice != PersonMerge && it.Person.Choice != PersonSkip {
				r.people[it.Key] = -2
				return ResSkipped, "this source supplied only a numeric person id, which cannot prove a later read is the same person; choose merge or skip explicitly"
			}
		}
		for _, legacy := range it.LegacyKeys {
			if r.hasActiveMark(legacy) && it.Person.Role != "owner" && it.Person.Choice != PersonMerge && it.Person.Choice != PersonSkip {
				r.people[it.Key] = -2
				return ResSkipped, "an older move used only this source's numeric id, which cannot prove this is the same person; choose merge or skip explicitly"
			}
		}
	}
	if id, ok := r.madeItem(it); ok && group != GroupRules && it.Ask == nil &&
		(it.Person == nil || it.Person.StableIdentity && it.Person.Choice != PersonSkip && it.Person.Choice != PersonMerge && it.Person.Role != "owner") {
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

type settingValue struct {
	Exists bool   `json:"exists"`
	Value  string `json:"value"`
}

type askBaseline struct {
	UserID          int64  `json:"user_id"`
	AskedAt         string `json:"asked_at"`
	UserFingerprint string `json:"user_fingerprint"`
}

type askPersonMark struct {
	AskID           int64  `json:"ask_id"`
	UserID          int64  `json:"user_id"`
	AskedAt         string `json:"asked_at"`
	AskFingerprint  string `json:"ask_fingerprint"`
	UserFingerprint string `json:"user_fingerprint"`
}

func askIdentity(ctx context.Context, q rowQuerier, id int64) (string, error) {
	var fp string
	err := q.QueryRowContext(ctx, "SELECT created_at || char(31) || kind || char(31) || ref FROM asks WHERE id = ?", id).Scan(&fp)
	return fp, err
}

func userIdentity(ctx context.Context, q rowQuerier, id int64) (string, error) {
	var fp string
	err := q.QueryRowContext(ctx, "SELECT created_at FROM users WHERE id = ?", id).Scan(&fp)
	return fp, err
}

func (r *run) expectedUserIdentity(key string, id int64) (string, error) {
	if key != "" {
		var row int64
		var fp string
		err := r.m.Store.Read().QueryRowContext(r.ctx, `SELECT m.row_id,m.fingerprint FROM move_made m JOIN moves v ON v.id=m.move_id
			WHERE m.key=? AND m.what='user' AND v.state!='undone' ORDER BY m.rowid DESC LIMIT 1`, key).Scan(&row, &fp)
		if err == nil && row == id {
			if at := strings.IndexByte(fp, 31); at > 0 {
				return fp[:at], nil
			}
			return "", errUndoReplaced
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return userIdentity(r.ctx, r.m.Store.Read(), id)
}

func (r *run) askMarkThisMove(key string) (bool, bool) {
	tx, err := r.m.Store.Read().BeginTx(r.ctx, nil)
	if err != nil {
		return false, false
	}
	defer tx.Rollback()
	var x madeRow
	err = tx.QueryRowContext(r.ctx, "SELECT key,what,row_id,before,after,fingerprint FROM move_made WHERE move_id=? AND key=?", r.move, key).
		Scan(&x.key, &x.what, &x.row, &x.before, &x.after, &x.fp)
	if err != nil {
		return false, false
	}
	switch x.what {
	case "ask":
		var b askBaseline
		if json.Unmarshal([]byte(x.after), &b) != nil || b.UserFingerprint == "" || fingerprint(r.ctx, tx, "ask", x.row) != x.fp {
			return true, false
		}
		userFP, err := userIdentity(r.ctx, tx, b.UserID)
		if err != nil || userFP != b.UserFingerprint {
			return true, false
		}
		var one int
		err = tx.QueryRowContext(r.ctx, "SELECT 1 FROM ask_people WHERE ask_id=? AND user_id=? AND asked_at=? AND counted=0", x.row, b.UserID, b.AskedAt).Scan(&one)
		return true, err == nil
	case "ask_person", "ask_claim":
		var b askPersonMark
		if json.Unmarshal([]byte(x.before), &b) != nil || b.AskFingerprint == "" || b.UserFingerprint == "" {
			return true, false
		}
		askFP, askErr := askIdentity(r.ctx, tx, b.AskID)
		userFP, userErr := userIdentity(r.ctx, tx, b.UserID)
		if askErr != nil || userErr != nil || askFP != b.AskFingerprint || userFP != b.UserFingerprint {
			return true, false
		}
		var one int
		err = tx.QueryRowContext(r.ctx, "SELECT 1 FROM ask_people WHERE ask_id=? AND user_id=? AND asked_at=? AND counted=0", b.AskID, b.UserID, b.AskedAt).Scan(&one)
		return true, err == nil
	default:
		return true, false
	}
}

type askSettingsMark map[string]settingValue

func readSettingValue(ctx context.Context, q rowQuerier, key string) (settingValue, error) {
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return settingValue{}, nil
	} else if err != nil {
		return settingValue{}, err
	}
	return settingValue{Exists: true, Value: raw}, nil
}

func (r *run) askSettings(it Item) (string, string) {
	cur, err := asks.ReadSettings(r.ctx, r.m.Store.Read())
	if err != nil {
		return ResFailed, err.Error()
	}
	next := cur
	if it.AskSettings.MoviesPerWeek != nil {
		next.MoviesPerWeek = *it.AskSettings.MoviesPerWeek
	}
	if it.AskSettings.ShowsPerWeek != nil {
		next.ShowsPerWeek = *it.AskSettings.ShowsPerWeek
	}
	if next.MoviesPerWeek == cur.MoviesPerWeek && next.ShowsPerWeek == cur.ShowsPerWeek {
		return ResAlready, "these are already the default weekly limits"
	}
	if r.dry {
		return ResChange, "default movie and show weekly limits"
	}
	wanted := map[string]*int{
		"ask_movies_per_week": it.AskSettings.MoviesPerWeek,
		"ask_shows_per_week":  it.AskSettings.ShowsPerWeek,
	}
	who, _ := json.Marshal(r.by)
	err = r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		beforeState, afterState := askSettingsMark{}, askSettingsMark{}
		for key, value := range wanted {
			if value == nil {
				continue
			}
			state, err := readSettingValue(r.ctx, tx, key)
			if err != nil {
				return err
			}
			beforeState[key] = state
			afterState[key] = settingValue{Exists: true, Value: strconv.Itoa(*value)}
		}
		now := store.FormatTime(r.m.now())
		for k, state := range afterState {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, state.Value, now, string(who), now, string(who)); err != nil {
				return err
			}
		}
		before, _ := json.Marshal(beforeState)
		after, _ := json.Marshal(afterState)
		return r.markTx(tx, it.Key, "ask_settings", 0, string(before), string(after))
	})
	return result(err, ResChanged, "default movie and show weekly limits")
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

// seededWait is the protocol block the ready-made Pick rules come with.
var seededWait = domain.ProtocolRule{Prefer: "either"}

// untouchedWait says a protocol block is still as it came.
func untouchedWait(p domain.ProtocolRule) bool {
	if p.Prefer == "" {
		p.Prefer = "either"
	}
	return p == seededWait
}

// protocol sets an old app's default wait on ready-made Pick rules, only
// while their wait is still as it came; the old block is kept for undo.
func (r *run) protocol(it Item) (string, string) {
	pp := it.Protocol
	id := r.id("SELECT id FROM pick_rules WHERE name = ? AND preset = 1", pp.Preset)
	if id == 0 {
		return ResNotFound, "the ready-made Pick rules " + pp.Preset + " are not here any more"
	}
	var cur domain.PickRules
	if err := store.Get(r.ctx, r.m.Store.Read(), "pick_rules", id, &cur); err != nil {
		return ResFailed, err.Error()
	}
	want := domain.ProtocolRule{Prefer: pp.Prefer, WaitMinutesUsenet: pp.WaitUsenet, WaitMinutesTorrent: pp.WaitTorrent, SkipWaitAtGoodEnough: pp.SkipAtGoodEnough}
	switch {
	case cur.Body.Protocol == want:
		return ResAlready, pp.Preset + " already waits this way"
	case !untouchedWait(cur.Body.Protocol):
		return ResKept, "this server's Pick rules already set a wait, so it stays"
	}
	what := pp.Preset + ": " + waitWords(*pp)
	if r.dry {
		u, err := library.PickRulesUse(r.ctx, r.m.Store, id)
		if err != nil {
			return ResFailed, err.Error()
		}
		return ResChange, fmt.Sprintf("%s; used by %d versions here now", what, u.Versions)
	}
	before, _ := json.Marshal(cur.Body.Protocol)
	after, _ := json.Marshal(want)
	body := cur.Body
	body.Protocol = want
	_, err := library.UpdatePickRules(r.ctx, r.m.Store, id, library.PickRulesInput{Name: cur.Name, Kinds: cur.Kinds, Body: body}, r.by, func(tx *sql.Tx) error {
		// Changed by someone else since it was read: leave it to the next move.
		var rev int
		if err := tx.QueryRow("SELECT revision FROM pick_rules WHERE id = ?", id).Scan(&rev); err != nil {
			return err
		}
		if rev != cur.Revision+1 {
			return errors.New("these Pick rules changed during the move; move again")
		}
		return r.markTx(tx, it.Key, "pick_rules_protocol", id, string(before), string(after))
	})
	return result(err, ResChanged, what)
}

// outsideList adds a Radarr list as a paused Outside list.
func (r *run) outsideList(it Item) (string, string) {
	o := it.OutsideList
	if r.id("SELECT id FROM outside_lists WHERE kind = ? AND ref = ? COLLATE NOCASE", o.Kind, o.Ref) != 0 {
		return ResAlready, "an Outside list for this list is here"
	}
	lib := filepath.Clean(filepath.FromSlash(o.Library))
	what := map[string]string{lists.KindTMDB: "TMDB list ", lists.KindTrakt: "Trakt list "}[o.Kind] + o.Ref + " into " + o.Library + ", paused until you check it and turn it on"
	if r.dry {
		if r.id("SELECT id FROM library_folders WHERE path = ?", lib) == 0 && !isDir(lib) {
			return ResNotFound, "no folder at " + o.Library + " on this server; add a path rule"
		}
		return ResCreate, what
	}
	lfID := r.id("SELECT id FROM library_folders WHERE path = ? AND kind = 'movie'", lib)
	if lfID == 0 {
		return ResNotFound, "its Library folder " + o.Library + " is not here (not moved, or left out)"
	}
	name := freeListName(it.Name, func(n string) bool {
		return r.id("SELECT id FROM outside_lists WHERE name = ? COLLATE NOCASE", n) != 0
	})
	off, keep := false, o.KeepUpToDate
	in := lists.Input{Name: name, Kind: o.Kind, Ref: o.Ref, LibraryFolderID: lfID, PickRulesID: r.presetID(o.Preset, "movie"), KeepUpToDate: &keep,
		SearchOnAdd: o.SearchOnAdd, ReleaseGate: o.ReleaseGate, Enabled: &off, SyncEveryHours: o.SyncEveryHours}
	_, err := lists.Add(r.ctx, r.m.Store, in, r.by, r.markWith(it.Key, "outside_list"))
	return result(err, ResCreated, what)
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
		return ResAdopt, "adopted in place from " + filepath.ToSlash(dir) + "; no file is moved or renamed"
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
		if _, removeErr := r.m.Library.Remove(context.WithoutCancel(r.ctx), id, library.RemoveChoice{}, true, r.by); removeErr != nil && !errors.Is(removeErr, store.ErrNotFound) && !errors.Is(removeErr, library.ErrAlreadyRemoved) {
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
		if _, removeErr := r.m.Library.Remove(context.WithoutCancel(r.ctx), id, library.RemoveChoice{}, true, r.by); removeErr != nil && !errors.Is(removeErr, store.ErrNotFound) && !errors.Is(removeErr, library.ErrAlreadyRemoved) {
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
	now := store.FormatTime(r.m.now())
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
		PathMap: []domain.PathMap{}, AfterImport: "leave", State: "disabled", Username: d.Username, AllowLocal: false, ClearFinished: true}
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
		if pp.Email != "" || pp.MoviesPerWeek != nil || pp.ShowsPerWeek != nil {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupPeople, From: it.From, What: it.Name + ": contact and ask limits",
				Why: "the person was matched to the existing HolyArrs owner, so the owner's current email and limits are kept"})
		}
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
		if pp.Email != "" || pp.MoviesPerWeek != nil || pp.ShowsPerWeek != nil {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupPeople, From: it.From, What: it.Name + ": contact and ask limits",
				Why: "the person was merged with an existing HolyArrs account, so its current email and limits are kept"})
		}
		r.people[it.Key] = id
		return ResMatched, "merged with the person of this name here, as you chose; their asks are theirs"
	}
	if pp.Choice == PersonMerge {
		return ResSkipped, "the person chosen for merge is no longer here; preview again and choose merge or skip"
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
		if _, err = tx.Exec("INSERT INTO user_ask_settings (user_id, per_week, movies_per_week, shows_per_week, email) VALUES (?, NULL, ?, ?, ?)",
			id, pp.MoviesPerWeek, pp.ShowsPerWeek, email); err != nil {
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
	if allowed, planned := r.personMarked[key]; planned && !allowed {
		return 0
	}
	if stable, planned := r.personStable[key]; planned && !stable {
		if id, ok := r.madeThisMove(key); ok {
			return id
		}
		return 0
	}
	if id, ok := r.made(key); ok {
		return id
	}
	return 0
}

func (r *run) createAsk(it Item, row *domain.Ask, by int64, asked time.Time, expectedAskerFP string, decidedBy int64, expectedDeciderFP string) (int64, error) {
	var id int64
	err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
		askerFP, err := userIdentity(r.ctx, tx, by)
		if err != nil || askerFP != expectedAskerFP {
			return errUndoReplaced
		}
		if decidedBy > 0 {
			deciderFP, err := userIdentity(r.ctx, tx, decidedBy)
			if err != nil || deciderFP != expectedDeciderFP {
				return errUndoReplaced
			}
		}
		if id, err = store.Insert(tx, "asks", row, r.by); err != nil {
			return err
		}
		// A moved ask never counts against the person's weekly limit.
		if _, err = tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)", id, by, store.FormatTime(asked)); err != nil {
			return err
		}
		baseline, _ := json.Marshal(askBaseline{UserID: by, AskedAt: store.FormatTime(asked), UserFingerprint: askerFP})
		return r.markTx(tx, it.Key, "ask", id, "", string(baseline))
	})
	return id, err
}

func (r *run) ask(it Item) (string, string) {
	a := it.Ask
	if a.Kind == "show" && !r.m.partOn("shows") || a.Kind == "movie" && !r.m.partOn("movies") {
		return ResSkipped, "its part is off"
	}
	tmdb := a.IDs["tmdb"]
	existing := r.id(`SELECT id FROM asks WHERE kind = ? AND state IN ('asked', 'approved') AND (ref = ? OR (tmdb_id != '' AND tmdb_id = ?))`, a.Kind, a.Ref, tmdb)
	var titleID int64
	if a.Kind == "movie" {
		titleID, _ = library.TitleByTMDB(r.ctx, r.m.Store.Read(), tmdb)
	} else {
		titleID, _ = library.ShowByIDs(r.ctx, r.m.Store.Read(), a.IDs)
	}
	by := r.personID(a.By)
	if by == -2 {
		return ResSkipped, "the request owner has an unverifiable older identity; choose merge or skip for that person before moving this request"
	}
	note := a.Note
	if by == 0 {
		by = r.owner
		if a.ByName != "" {
			note = strings.TrimSpace(note + "; asked by " + a.ByName + " in " + a.App)
		}
	}
	if existing != 0 {
		expectedAskFP, askIdentityErr := askIdentity(r.ctx, r.m.Store.Read(), existing)
		expectedUserFP, userIdentityErr := r.expectedUserIdentity(a.By, by)
		if askIdentityErr != nil || userIdentityErr != nil {
			return ResFailed, "the existing request owner identity could not be verified"
		}
		if a.Approved || a.Decider != "" || a.DecidedAt != "" {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupWishlist, From: it.From, What: it.Name + ": old decision",
				Why: "Requests entry already exists, so its current decision and decision time are kept"})
		}
		if by > 0 && r.id("SELECT ask_id FROM ask_people WHERE ask_id = ? AND user_id = ?", existing, by) != 0 {
			if !r.dry {
				err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
					askFP, err := askIdentity(r.ctx, tx, existing)
					if err != nil || askFP != expectedAskFP {
						return errUndoReplaced
					}
					userFP, err := userIdentity(r.ctx, tx, by)
					if err != nil || userFP != expectedUserFP {
						return errUndoReplaced
					}
					var askedAt string
					if err := tx.QueryRowContext(r.ctx, "SELECT asked_at FROM ask_people WHERE ask_id = ? AND user_id = ? AND counted=0", existing, by).Scan(&askedAt); err != nil {
						return err
					}
					mark, _ := json.Marshal(askPersonMark{AskID: existing, UserID: by, AskedAt: askedAt, AskFingerprint: askFP, UserFingerprint: userFP})
					return r.markTx(tx, it.Key, "ask_claim", existing, string(mark), "")
				})
				if err != nil {
					return ResFailed, err.Error()
				}
			}
			return ResAlready, "already on the Requests page for this person"
		}
		if r.dry {
			return ResCreate, "adds this person's request to the existing request"
		}
		if by <= 0 {
			return ResFailed, "its asker was not moved"
		}
		asked, timeErr := r.sourceOrMoveTime(a.AskedAt)
		if timeErr != nil {
			return ResFailed, "the move's stable request time was not found: " + timeErr.Error()
		}
		err := r.m.Store.Write(r.ctx, func(tx *sql.Tx) error {
			askFP, err := askIdentity(r.ctx, tx, existing)
			if err != nil || askFP != expectedAskFP {
				return errUndoReplaced
			}
			userFP, err := userIdentity(r.ctx, tx, by)
			if err != nil || userFP != expectedUserFP {
				return errUndoReplaced
			}
			if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)", existing, by, store.FormatTime(asked)); err != nil {
				return err
			}
			mark, _ := json.Marshal(askPersonMark{AskID: existing, UserID: by, AskedAt: store.FormatTime(asked), AskFingerprint: askFP, UserFingerprint: userFP})
			return r.markTx(tx, it.Key, "ask_person", 0, string(mark), "")
		})
		return result(err, ResCreated, "added this person's request to the existing request")
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
	expectedAskerFP, identityErr := r.expectedUserIdentity(a.By, by)
	if identityErr != nil || expectedAskerFP == "" {
		return ResFailed, "the request owner identity could not be verified"
	}
	asked, timeErr := r.sourceOrMoveTime(a.AskedAt)
	if timeErr != nil {
		return ResFailed, "the move's stable request time was not found: " + timeErr.Error()
	}
	row := domain.Ask{Kind: a.Kind, TMDBID: tmdb, Name: it.Name, State: state, AskedBy: by, AskedAt: asked, Note: clip(note, 300),
		Ref: a.Ref, SourceIDs: a.IDs, Seasons: append([]int{}, a.Seasons...), Albums: []domain.AskAlbum{}}
	if a.Kind == "movie" {
		row.Seasons = []int{}
	}
	if titleID != 0 {
		row.TitleID = &titleID
	}
	var decidedBy int64
	var expectedDeciderFP string
	if state == "approved" {
		decidedBy = r.personID(a.Decider)
		if decidedBy <= 0 {
			r.rep.NotMoved = append(r.rep.NotMoved, NotMoved{Group: GroupWishlist, From: it.From, What: it.Name + ": decision owner", Why: "the source approver could not be matched, so HolyArrs records the current owner as the approver"})
			decidedBy = r.owner
		}
		expectedDeciderFP, identityErr = r.expectedUserIdentity(a.Decider, decidedBy)
		if identityErr != nil || expectedDeciderFP == "" {
			return ResFailed, "the request decision owner identity could not be verified"
		}
		decided, timeErr := r.sourceOrMoveTime(a.DecidedAt)
		if timeErr != nil {
			return ResFailed, "the move's stable decision time was not found: " + timeErr.Error()
		}
		row.DecidedBy, row.DecidedAt = &decidedBy, &decided
	}
	_, err := r.createAsk(it, &row, by, asked, expectedAskerFP, decidedBy, expectedDeciderFP)
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
	// This value is server-owned. Never let a caller supply the approval that
	// a queued/recovered job will trust.
	ch.ApprovedHash = ""
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
	ch.ApprovedHash = planHash
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
	// Only a move with a live job can resume. Orphaned moves keep the
	// interrupted-move report below instead of becoming approval failures.
	pending, err := moves(ctx, m.Store, `WHERE state IN ('queued', 'running')
		AND EXISTS (SELECT 1 FROM jobs j WHERE j.id = moves.job_id AND j.state IN ('queued', 'running'))`)
	if err != nil {
		return err
	}
	for _, mv := range pending {
		plan, planErr := LoadPlan(ctx, m.Store, mv.Snapshots, mv.Choices)
		if planErr == nil && (mv.Choices.ApprovedHash == "" || PlanHash(plan) != mv.Choices.ApprovedHash) {
			planErr = ErrPlanChanged
		}
		if planErr != nil {
			rep := &Report{Move: mv.ID, Lines: []Line{}, Counts: map[string]int{}, Next: []string{"This queued move no longer matches an approved preview. Preview it again and start a new move."}}
			if err := m.setState(ctx, mv.ID, "failed", rep); err != nil {
				return err
			}
		}
	}
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
	p, err := LoadPlan(ctx, m.Store, mv.Snapshots, mv.Choices)
	if err == nil && (mv.Choices.ApprovedHash == "" || PlanHash(p) != mv.Choices.ApprovedHash) {
		err = ErrPlanChanged
	}
	if err != nil {
		_ = m.setState(context.WithoutCancel(ctx), mv.ID, "failed", &Report{Move: mv.ID, Lines: []Line{}, Counts: map[string]int{},
			Next: []string{"The move could not start from its approved preview: " + err.Error() + ". Preview it again and start a new move."}})
		return err
	}
	if err := m.setState(ctx, mv.ID, "running", nil); err != nil {
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
	row, move                    int64
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
		x := madeRow{move: id}
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
	if rep.Counts[ResFailed] > 0 {
		rep.Next = append(rep.Next, "Some items could not be undone. Fix their reported error and retry this undo.")
		b, _ := json.Marshal(rep)
		err := m.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE moves SET undo_state = 'failed', report = ? WHERE id = ? AND state != 'undone'", string(b), id)
			return err
		})
		if err != nil {
			return rep, err
		}
		return rep, errUndoIncomplete
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

var (
	errUndoChanged    = errors.New("changed since the move")
	errUndoGone       = errors.New("already gone")
	errUndoReplaced   = errors.New("move row was replaced")
	errUndoIncomplete = errors.New("some moved items could not be undone")
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
		del := func(extra, why string, extraArgs ...any) (string, string) {
			args := append([]any{x.row, x.fp}, extraArgs...)
			c, err := exec("DELETE FROM "+n[0]+" WHERE id = ? AND created_at || char(31) || "+n[1]+" = ?"+extra, args...)
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
			var baseline askBaseline
			if json.Unmarshal([]byte(x.after), &baseline) != nil || baseline.UserID <= 0 || baseline.AskedAt == "" || baseline.UserFingerprint == "" {
				return ResKept, keptUnknown
			}
			return del(` AND state IN ('asked', 'approved')
				AND (SELECT count(*) FROM ask_people WHERE ask_id = asks.id) = 1
				AND EXISTS (SELECT 1 FROM ask_people WHERE ask_id = asks.id AND user_id = ? AND asked_at = ? AND counted = 0)
				AND EXISTS (SELECT 1 FROM users WHERE id = ? AND created_at = ?)
				AND NOT EXISTS (SELECT 1 FROM move_made mm JOIN moves mv ON mv.id = mm.move_id
					WHERE mm.what = 'ask_claim' AND mm.row_id = asks.id AND mv.state != 'undone' AND mm.move_id != ?)`,
				"another person joined this ask or its ownership changed since, so it stays", baseline.UserID, baseline.AskedAt, baseline.UserID, baseline.UserFingerprint, x.move)
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
		case "outside_list":
			return del(" AND enabled = 0", "you turned it on since, so it stays; remove it under Settings > Outside lists")
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
	case "music_policy":
		if x.fp == "" {
			return ResKept, keptUnknown
		}
		var old, moved musicState
		if json.Unmarshal([]byte(x.before), &old) != nil || json.Unmarshal([]byte(x.after), &moved) != nil {
			return ResKept, keptUnknown
		}
		err := m.Store.Write(ctx, func(tx *sql.Tx) error {
			return undoMusicPolicyTx(ctx, tx, x, old, moved, by, store.FormatTime(m.now()))
		})
		switch {
		case errors.Is(err, errUndoGone):
			return ResKept, "already gone"
		case errors.Is(err, errUndoReplaced):
			return ResKept, keptReplaced
		case errors.Is(err, errUndoChanged):
			return ResKept, keptChanged
		}
		if err != nil {
			return ResFailed, err.Error()
		}
		return ResChanged, "artist and album choices restored"
	case "ask_person":
		var mark askPersonMark
		if json.Unmarshal([]byte(x.before), &mark) != nil || mark.AskID <= 0 || mark.UserID <= 0 || mark.AskedAt == "" || mark.AskFingerprint == "" || mark.UserFingerprint == "" {
			return ResKept, keptUnknown
		}
		var n int64
		err := m.Store.Write(ctx, func(tx *sql.Tx) error {
			askFP, err := askIdentity(ctx, tx, mark.AskID)
			if errors.Is(err, sql.ErrNoRows) || askFP != mark.AskFingerprint {
				return errUndoReplaced
			}
			if err != nil {
				return err
			}
			userFP, err := userIdentity(ctx, tx, mark.UserID)
			if errors.Is(err, sql.ErrNoRows) || userFP != mark.UserFingerprint {
				return errUndoReplaced
			}
			if err != nil {
				return err
			}
			res, err := tx.Exec(`DELETE FROM ask_people WHERE ask_id = ? AND user_id = ? AND asked_at = ? AND counted = 0
				AND NOT EXISTS (SELECT 1 FROM move_made mm JOIN moves mv ON mv.id=mm.move_id
					WHERE mm.what='ask_claim' AND mv.state!='undone' AND mm.row_id=? AND mm.move_id != ?)`,
				mark.AskID, mark.UserID, mark.AskedAt, mark.AskID, x.move)
			if err == nil {
				n, _ = res.RowsAffected()
			}
			return err
		})
		if errors.Is(err, errUndoReplaced) {
			return ResKept, keptReplaced
		}
		if err != nil {
			return ResFailed, err.Error()
		}
		if n == 0 {
			return ResKept, "already gone or changed"
		}
		return ResRemoved, "request ownership link removed"
	case "ask_claim":
		return ResRemoved, "later move's Requests claim released; the existing request and ownership stay"
	case "ask_settings":
		var old, moved askSettingsMark
		if json.Unmarshal([]byte(x.before), &old) != nil || json.Unmarshal([]byte(x.after), &moved) != nil {
			return ResKept, keptUnknown
		}
		err := m.Store.Write(ctx, func(tx *sql.Tx) error {
			return undoAskSettingsTx(ctx, tx, old, moved, by, store.FormatTime(m.now()))
		})
		if errors.Is(err, errUndoChanged) {
			return ResKept, keptChanged
		}
		if err != nil {
			return ResFailed, err.Error()
		}
		return ResRemoved, "the default weekly limits are back as they were"
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
	case "pick_rules_protocol":
		var old, moved domain.ProtocolRule
		if x.after == "" || json.Unmarshal([]byte(x.before), &old) != nil || json.Unmarshal([]byte(x.after), &moved) != nil {
			return ResKept, keptUnknown
		}
		var cur domain.PickRules
		if err := store.Get(ctx, m.Store.Read(), "pick_rules", x.row, &cur); errors.Is(err, store.ErrNotFound) {
			return ResKept, "already gone"
		} else if err != nil {
			return ResFailed, err.Error()
		}
		if cur.Body.Protocol != moved {
			return ResKept, keptChanged
		}
		body := cur.Body
		body.Protocol = old
		_, err := library.UpdatePickRules(ctx, m.Store, x.row, library.PickRulesInput{Name: cur.Name, Kinds: cur.Kinds, Body: body}, by, func(tx *sql.Tx) error {
			var rev int
			if err := tx.QueryRow("SELECT revision FROM pick_rules WHERE id = ?", x.row).Scan(&rev); err != nil {
				return err
			}
			if rev != cur.Revision+1 {
				return errUndoChanged
			}
			return nil
		})
		if errors.Is(err, errUndoChanged) {
			return ResKept, keptChanged
		}
		if err != nil {
			return ResFailed, err.Error()
		}
		return ResRemoved, "the wait on " + cur.Name + " is back as it was"
	}
	return ResKept, "nothing to undo for it"
}
