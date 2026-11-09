package main

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
)

// Undo changes the Library, so safe mode refuses it too.
func TestMoveUndoRefusedInSafeMode(t *testing.T) {
	t.Parallel()
	srv, st, _, _ := moveSetup(t, true)
	ctx := context.Background()
	var id int64
	err := st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO moves (created_at, owner_id, state, job_id, undo_state) VALUES (?, 1, 'done', NULL, '')`,
			database.FormatTime(time.Now().UTC()))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b := lbDo(t, "POST", srv.URL+"/api/v1/migrate/moves/"+lbItoa(id)+"/undo", "", 409)
	if !strings.Contains(strings.ToLower(string(b)), "safe mode") {
		t.Fatalf("body %s", b)
	}
	var undo string
	if err := st.Read().QueryRowContext(ctx, `SELECT undo_state FROM moves WHERE id = ?`, id).Scan(&undo); err != nil {
		t.Fatal(err)
	}
	if undo != "" {
		t.Fatalf("undo_state %q", undo)
	}
}

// Safe mode set through parts.Control (the restart path) also stops moves
// and turns every part off for the mover.
func TestMoveRefusedWhenPartsSafeMode(t *testing.T) {
	t.Parallel()
	_, st, snap, films := moveSetup(t, false)
	q := jobs.New(st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	lib := &library.Service{Store: st, Queue: q, Metadata: metadata.Lab()}
	mv := &migrate.Mover{Store: st, Library: lib, Queue: q}
	h := buildHandlers(services{Store: st, Queue: q, Library: lib, Metadata: metadata.Lab(), Version: "test", Mover: mv,
		Parts: &parts.Control{Store: st, SafeMode: true}})
	srv := httptest.NewServer(h.Handler())
	t.Cleanup(srv.Close)
	lbDo(t, "POST", srv.URL+"/api/v1/migrate/moves", moveBody(snap, films), 409)
	if h.mover().PartOn("movies") {
		t.Fatal("movies on in safe mode")
	}
}
