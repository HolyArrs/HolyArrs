package requests

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// readdTitle deletes a title the way the Library does (the row stays,
// marked removed) and adds the same title back as a new row.
func readdTitle(t *testing.T, r *rig, old int64) int64 {
	t.Helper()
	var cols []string
	rows, err := r.st.Read().Query("SELECT name FROM pragma_table_info('titles') WHERE name != 'id'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		cols = append(cols, c)
	}
	rows.Close()
	list := strings.Join(cols, ", ")
	var id int64
	err = r.st.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", old); err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO titles ("+list+") SELECT "+list+" FROM titles WHERE id = ?", old)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE titles SET state = 'active' WHERE id = ?", id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func askTitle(t *testing.T, r *rig, id int64) sql.NullInt64 {
	t.Helper()
	var tid sql.NullInt64
	if err := r.st.Read().QueryRow("SELECT title_id FROM asks WHERE id = ?", id).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	return tid
}

func TestReaddedTitleRelinksItsAsks(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || a.TitleID == nil {
		t.Fatalf("approve %+v %v", a, err)
	}
	other, _ := r.svc.Ask(ctx, r.ana, "900002")
	setState(t, r, other.Ask.ID, "turned_down")

	nid := readdTitle(t, r, *a.TitleID)
	vs, _ := r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
	if len(vs) != 1 || vs[0].Problem == "" {
		t.Fatalf("before relink the ask should read as failed: %+v", vs)
	}
	n, err := r.svc.RelinkTitle(ctx, nid)
	if err != nil || n != 1 {
		t.Fatalf("relink %d %v", n, err)
	}
	if got := askTitle(t, r, res.Ask.ID); !got.Valid || got.Int64 != nid {
		t.Fatalf("ask title %v, want %d", got, nid)
	}
	vs, _ = r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
	if len(vs) != 1 || vs[0].Problem != "" {
		t.Fatalf("after relink: %+v", vs)
	}
	if got := askTitle(t, r, other.Ask.ID); got.Valid {
		t.Fatalf("turned-down ask of another title linked to %v", got)
	}
	// Run again: nothing left to move, and a live link is never taken.
	if n, err := r.svc.RelinkTitle(ctx, *a.TitleID); err != nil || n != 0 {
		t.Fatalf("removed title took asks: %d %v", n, err)
	}
	if n, err := r.svc.RelinkAll(ctx); err != nil || n != 0 {
		t.Fatalf("second pass: %d %v", n, err)
	}
}

func TestRelinkAllOnStartFixesOldAsks(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ready, _ := r.svc.Ask(ctx, r.bo, "900003")
	setState(t, r, ready.Ask.ID, "available") // no title at all
	declined, _ := r.svc.Ask(ctx, r.bo, "900005")
	setState(t, r, declined.Ask.ID, "turned_down")
	nid := readdTitle(t, r, *a.TitleID)
	n, err := r.svc.RelinkAll(ctx)
	if err != nil || n != 1 {
		t.Fatalf("relink all %d %v", n, err)
	}
	if got := askTitle(t, r, res.Ask.ID); got.Int64 != nid {
		t.Fatalf("ask title %v, want %d", got, nid)
	}
	if askTitle(t, r, ready.Ask.ID).Valid || askTitle(t, r, declined.Ask.ID).Valid {
		t.Fatal("asks without a matching Library title were linked")
	}
}
