package requests

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func askCount(t *testing.T, r *rig, where string) int {
	t.Helper()
	var n int
	if err := r.st.Read().QueryRow("SELECT count(*) FROM asks WHERE " + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func setState(t *testing.T, r *rig, id int64, state string) {
	t.Helper()
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET state = ? WHERE id = ?", state, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoveAnyStateKeepsTitleAndWritesHistory(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	_, _ = r.svc.Ask(ctx, r.bo, "693134")
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	var titles int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM titles").Scan(&titles)
	if err := r.svc.Remove(ctx, res.Ask.ID, r.luis); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM titles").Scan(&after)
	if titles == 0 || after != titles {
		t.Fatalf("titles %d -> %d", titles, after)
	}
	if askCount(t, r, "id = "+strconv.FormatInt(res.Ask.ID, 10)) != 0 {
		t.Fatal("ask still there")
	}
	var people int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM ask_people WHERE ask_id = ?", res.Ask.ID).Scan(&people)
	if people != 0 {
		t.Fatalf("people left %d", people)
	}
	found := false
	for _, e := range r.events(t) {
		if e.Kind == domain.EventAskWithdrawn && strings.Contains(e.Summary, "luis removed the request for") {
			found = true
		}
	}
	if !found {
		t.Fatal("no History line")
	}
	if err := r.svc.Remove(ctx, res.Ask.ID, r.luis); err == nil {
		t.Fatal("removed twice")
	}
}

func TestRemoveEachState(t *testing.T) {
	for _, state := range []string{"asked", "approved", "turned_down", "available"} {
		r := newRig(t)
		ctx := context.Background()
		res, _ := r.svc.Ask(ctx, r.ana, "693134")
		setState(t, r, res.Ask.ID, state)
		if err := r.svc.Remove(ctx, res.Ask.ID, r.luis); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
	}
}

func TestMemberCannotWithdrawAfterAnswer(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	setState(t, r, res.Ask.ID, "turned_down")
	if err := r.svc.Withdraw(ctx, res.Ask.ID, r.ana, false); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("own answered: %v", err)
	}
	if err := r.svc.Withdraw(ctx, res.Ask.ID, r.bo, false); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("other's answered: %v", err)
	}
	if askCount(t, r, "1=1") != 1 {
		t.Fatal("ask gone")
	}
}

func TestRemoveManyByIDsAndFilters(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002", "900003", "900005"} {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.Ask.ID)
	}
	setState(t, r, ids[0], "approved") // no title: failed
	setState(t, r, ids[1], "turned_down")
	setState(t, r, ids[2], "available")
	n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed)
	if err != nil || n != 1 || askCount(t, r, "state = 'asked'") != 1 || askCount(t, r, "1=1") != 3 {
		t.Fatalf("failed: %d %v", n, err)
	}
	n, err = r.svc.RemoveMany(ctx, r.luis, nil, RemoveFinished)
	if err != nil || n != 2 || askCount(t, r, "1=1") != 1 {
		t.Fatalf("finished: %d %v", n, err)
	}
	n, err = r.svc.RemoveMany(ctx, r.luis, []int64{ids[3], 9999}, "")
	if err != nil || n != 1 || askCount(t, r, "1=1") != 0 {
		t.Fatalf("ids: %d %v", n, err)
	}
	if _, err := r.svc.RemoveMany(ctx, r.luis, nil, "nope"); err == nil {
		t.Fatal("bad filter accepted")
	}
}
