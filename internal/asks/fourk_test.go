package asks

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// Roadmap #59: a 4K ask is its own ask, approved apart from the plain one,
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
		WHERE v.title_id = ? AND v.label = '4K' AND p.name = ?`, *got.TitleID, domain.FourKName).Scan(&n)
	if n != 1 {
		t.Fatalf("4K versions %d", n)
	}
	// With a 4K Version there, a new 4K ask finds it in the Library.
	_ = r.st.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("UPDATE asks SET state = 'available' WHERE id = ?", uhd.Ask.ID); return err })
	var in *InLibraryError
	if _, err := r.svc.Ask4K(ctx, r.ana, "693134"); !errors.As(err, &in) {
		t.Fatalf("4K again: %v", err)
	}
}
