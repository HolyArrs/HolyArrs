package requests

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A 4K ask is its own ask, approved apart from the plain one,
// and approving it adds a 4K Version on the 4K Pick rules.
func TestFourKAskAddsFourKVersion(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	plain, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	uhd, err := r.svc.Ask4K(ctx, r.bo, "693134")
	if err != nil || uhd.Joined || uhd.Ask.ID == plain.Ask.ID || !uhd.Ask.FourK {
		t.Fatalf("4K ask %+v %v", uhd, err)
	}
	vs, _ := r.svc.Views(ctx, "id = ?", uhd.Ask.ID)
	if len(vs) != 1 || !vs[0].FourK {
		t.Fatalf("view %+v", vs)
	}
	got, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// The plain approval leaves the 4K ask waiting.
	if a, _ := get(ctx, r.st.Read(), uhd.Ask.ID); a.State != domain.AskAsked {
		t.Fatalf("4K ask %+v", a)
	}
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.st.Read().QueryRow(`SELECT count(*) FROM versions v JOIN pick_rules p ON p.id = v.pick_rules_id
		WHERE v.title_id = ? AND v.label = '4K' AND p.name = ?`, *got.TitleID, mediafiles.FourKName).Scan(&n)
	if n != 1 {
		t.Fatalf("4K versions %d", n)
	}
	// With a 4K Version there, a new 4K ask finds it in the Library.
	_ = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET state = 'available' WHERE id = ?", uhd.Ask.ID)
		return err
	})
	var in *InLibraryError
	if _, err := r.svc.Ask4K(ctx, r.ana, "693134"); !errors.As(err, &in) {
		t.Fatalf("4K again: %v", err)
	}
}

// The member who asked for the regular copy may also ask for
// 4K, and a movie already in the Library without a 4K Version takes a 4K ask.
func TestFourKAskAfterOwnAskAndInLibrary(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	plain, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	uhd, err := r.svc.Ask4K(ctx, r.ana, "693134")
	if err != nil || uhd.Joined || uhd.Already || uhd.Ask.ID == plain.Ask.ID || !uhd.Ask.FourK {
		t.Fatalf("own 4K ask %+v %v", uhd, err)
	}
	// Turned down, the 4K ask is gone; the plain one is approved into the Library.
	if _, err := r.svc.TurnDown(ctx, uhd.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	if has, err := HasFourK(ctx, r.st.Read(), *got.TitleID); err != nil || has {
		t.Fatalf("has 4K %v %v", has, err)
	}
	// In the Library without 4K, another member's 4K ask is made.
	again, err := r.svc.Ask4K(ctx, r.bo, "693134")
	if err != nil || !again.Ask.FourK || again.Ask.State != domain.AskAsked {
		t.Fatalf("4K on owned %+v %v", again, err)
	}
	if _, err := r.svc.Approve(ctx, again.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if has, err := HasFourK(ctx, r.st.Read(), *got.TitleID); err != nil || !has {
		t.Fatalf("has 4K after approve %v %v", has, err)
	}
}
