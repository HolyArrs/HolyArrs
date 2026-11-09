package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// GET /migrate/moves/{id} reads the move's job for progress: the move job
// while it runs, the undo job while an undo runs (unless the move is done:
// then 1), 1 once done or undone, and 0 with no note when there is no job.
func TestRDPMoveProgressFromJobs(t *testing.T) {
	srv, st, _, _ := arrMoveSetup(t, false)
	ctx := context.Background()
	stamp := "2026-10-13T12:00:00Z"
	job := func(progress float64, note any) int64 {
		var id int64
		err := st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec(`INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, state, progress, progress_note, run_after, asked_by)
				VALUES (?, '{}', ?, '{}', 'rdp_pin_unhandled', 'running', ?, ?, ?, '{}')`, stamp, stamp, progress, note, stamp)
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	move := func(state, undo string, jobID, undoJob any) int64 {
		var id int64
		err := st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec(`INSERT INTO moves (created_at, owner_id, state, job_id, undo_state, undo_job_id) VALUES (?, 1, ?, ?, ?, ?)`, stamp, state, jobID, undo, undoJob)
			if err != nil {
				return err
			}
			id, _ = res.LastInsertId()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	_ = database.TimeLayout
	moveJob, undoJob := job(0.4, "adding titles"), job(0.7, nil)
	cases := []struct {
		name     string
		id       int64
		progress float64
		note     string
	}{
		{"running move", move("running", "", moveJob, nil), 0.4, "adding titles"},
		{"undo running on a done move reads 1", move("done", "running", moveJob, undoJob), 1, ""},
		{"undo queued on a done move reads 1", move("done", "queued", moveJob, undoJob), 1, ""},
		{"undo running on a running move reads the undo job", move("running", "running", moveJob, undoJob), 0.7, ""},
		{"done", move("done", "", moveJob, nil), 1, "adding titles"},
		{"undone", move("undone", "", moveJob, nil), 1, "adding titles"},
		{"no job yet", move("queued", "", nil, nil), 0, ""},
		{"job row gone", move("running", "", 424242, nil), 0, ""},
	}
	for _, c := range cases {
		var got struct {
			Progress     float64 `json:"progress"`
			ProgressNote string  `json:"progress_note"`
		}
		if err := json.Unmarshal(arrDo(t, "GET", srv.URL+"/api/v1/migrate/moves/"+arrItoa(c.id), "", 200), &got); err != nil {
			t.Fatal(err)
		}
		if got.Progress != c.progress || got.ProgressNote != c.note {
			t.Errorf("%s: progress %v note %q, want %v %q", c.name, got.Progress, got.ProgressNote, c.progress, c.note)
		}
	}
}
