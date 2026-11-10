package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

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
			_, err := tx.Exec("UPDATE moves SET state = 'undone', undo_state = '', report = ?, undone_at = ? WHERE id = ?", string(b), database.FormatTime(m.now()), id)
			return err
		}
		if _, err := tx.Exec("UPDATE moves SET state = ?, report = ?, finished_at = ? WHERE id = ?", state, string(b), database.FormatTime(m.now()), id); err != nil {
			return err
		}
		if state == "done" && (rep == nil || rep.Counts[ResFailed] == 0) {
			// A finished move forgets every arr key kept for "Read
			// again" (the move credentials and their vault rows).
			// While any item failed (say the metadata server was down) the
			// keys stay, so "import again" needs no retyping; the next clean
			// import forgets them.
			if _, err := tx.Exec("DELETE FROM secrets WHERE id IN (SELECT secret_id FROM move_credentials)"); err != nil {
				return err
			}
			if _, err := tx.Exec("DELETE FROM move_credentials"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *Mover) setUndoState(ctx context.Context, id int64, state string) error {
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET undo_state = ? WHERE id = ?", state, id)
		return err
	})
}

// stoppedNote is the report of a move a stop cut off with no Job left.
const stoppedNote = "HolyArrs stopped during this import, before it finished. What it made is marked: import again to finish it (nothing is added twice), or undo it."

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
			rep := &Report{Move: mv.ID, Lines: []Line{}, Counts: map[string]int{}, Next: []string{"This queued import no longer matches an approved preview. Preview it again and start a new import."}}
			if err := m.setState(ctx, mv.ID, "failed", rep); err != nil {
				return err
			}
		}
	}
	return m.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE moves SET state = 'failed', report = CASE WHEN report IN ('', '{}') THEN ? ELSE report END, finished_at = ?
			WHERE state IN ('queued', 'running') AND (job_id IS NULL OR NOT EXISTS (SELECT 1 FROM jobs j WHERE j.id = moves.job_id AND j.state IN ('queued', 'running')))`,
			string(b), database.FormatTime(m.now()))
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
		if r.Stopping() {
			return ctx.Err() // a stop while reading the plan: resume after the restart
		}
		_ = m.setState(context.WithoutCancel(ctx), mv.ID, "failed", &Report{Move: mv.ID, Lines: []Line{}, Counts: map[string]int{},
			Next: []string{"The import could not start from its approved preview: " + err.Error() + ". Preview it again and start a new import."}})
		return err
	}
	if err := m.setState(ctx, mv.ID, "running", nil); err != nil {
		return err
	}
	by := r.Job.CreatedBy
	rep := m.Run(ctx, mv.ID, p, false, by, func(done, total int) {
		_ = r.Progress(ctx, float64(done)/float64(max(total, 1)), fmt.Sprintf("%d of %d imported", done, total))
	})
	if r.Stopping() {
		// The app is stopping (docker stop, an update): leave the move
		// running so it resumes after the restart, as after a hard kill.
		return ctx.Err()
	}
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
func GetMove(ctx context.Context, st *database.Store, id int64) (Move, error) {
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
func ListMoves(ctx context.Context, st *database.Store) ([]Move, error) {
	ms, err := moves(ctx, st, "ORDER BY id DESC LIMIT 100")
	for i := range ms {
		if ms[i].Report != nil {
			ms[i].Report = &Report{Counts: ms[i].Report.Counts, Lines: []Line{}, NotMoved: []NotMoved{}, NeedsInput: []string{}, Next: []string{}}
		}
	}
	return ms, err
}

func moves(ctx context.Context, st *database.Store, where string, args ...any) ([]Move, error) {
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
	// Each undo line keeps the name and group the move's report gave it.
	moved := map[string]Line{}
	if mv.Report != nil {
		for _, l := range mv.Report.Lines {
			moved[l.Key] = l
		}
	}
	for i, x := range made {
		if ctx.Err() != nil {
			return rep, ctx.Err()
		}
		res, detail := m.undoOne(ctx, x, by)
		line := Line{Group: x.what, Key: x.key, Name: x.key, Result: res, Detail: detail}
		if l, ok := moved[x.key]; ok {
			line.Group, line.Name = l.Group, l.Name
		}
		rep.Lines = append(rep.Lines, line)
		rep.Counts[res]++
		if progress != nil && ((i+1)%25 == 0 || i+1 == len(made)) {
			progress(i+1, len(made))
		}
	}
	rep.Next = []string{"Nothing in the old apps was touched by the import or by this undo, and no media file was moved or removed."}
	if rep.Counts[ResKept] > 0 {
		rep.Next = append(rep.Next, "Some things stay because you set them up or changed them after the import, or they are no longer the rows the import made; remove them by hand if you want.")
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
	keptReplaced = "this is no longer the row the import made (it was removed and its place reused, or renamed), so it stays"
	keptUnknown  = "it cannot be checked to be the row the import made (marked before this check existed), so it stays; remove it by hand if you want"
	keptChanged  = "you changed it since the import, so it stays as you left it"
)

var (
	errUndoChanged    = errors.New("changed since the import")
	errUndoGone       = errors.New("already gone")
	errUndoReplaced   = errors.New("import row was replaced")
	errUndoIncomplete = errors.New("some imported items could not be undone")
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
			if _, e := m.Library.Remove(ctx, x.row, library.RemoveChoice{}, true, by); e != nil && !errors.Is(e, database.ErrNotFound) && !errors.Is(e, library.ErrAlreadyRemoved) {
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
			case errors.Is(e, database.ErrNotFound):
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
			return undoMusicPolicyTx(ctx, tx, x, old, moved, by, database.FormatTime(m.now()))
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
		return ResRemoved, "later import's Requests claim released; the existing request and ownership stay"
	case "ask_settings":
		var old, moved askSettingsMark
		if json.Unmarshal([]byte(x.before), &old) != nil || json.Unmarshal([]byte(x.after), &moved) != nil {
			return ResKept, keptUnknown
		}
		err := m.Store.Write(ctx, func(tx *sql.Tx) error {
			return undoAskSettingsTx(ctx, tx, old, moved, by, database.FormatTime(m.now()))
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
		var old mediafiles.NamingSettings
		if x.after == "" || json.Unmarshal([]byte(x.after), &moved) != nil || json.Unmarshal([]byte(x.before), &old) != nil {
			return ResKept, keptUnknown
		}
		cur, err := mediafiles.ReadNaming(ctx, m.Store.Read())
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
		if err := mediafiles.PutNamingSettings(ctx, m.Store, old, by); err != nil {
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
		var old, moved mediafiles.ProtocolRule
		if x.after == "" || json.Unmarshal([]byte(x.before), &old) != nil || json.Unmarshal([]byte(x.after), &moved) != nil {
			return ResKept, keptUnknown
		}
		var cur mediafiles.PickRules
		if err := database.Get(ctx, m.Store.Read(), "pick_rules", x.row, &cur); errors.Is(err, database.ErrNotFound) {
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
