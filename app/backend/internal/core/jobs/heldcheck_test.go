package jobs

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// flipGate holds nothing when read on the read pool but holds every Job
// when asked again inside the claim's write: the part was turned off in
// between.
type flipGate struct{}

func (flipGate) Active() bool { return true }
func (flipGate) Held(_ context.Context, db database.Querier, _ string, _ *database.Subject) bool {
	_, inWrite := db.(*sql.Tx)
	return inWrite
}
func (flipGate) KindOff(string) bool { return false }

func TestGatedClaimChecksHeldInsideTheWrite(t *testing.T) {
	s := newStore(t, t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	q := New(s, Options{Workers: 1})
	q.SetGate(flipGate{})
	id, _, err := q.Enqueue(context.Background(), "x", nil, nil, "k", database.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := q.claim(context.Background(), ""); ok || err != nil {
		t.Fatalf("claimed a Job held inside the write: %v %v", ok, err)
	}
	var st string
	_ = s.Read().QueryRow("SELECT state FROM jobs WHERE id = ?", id).Scan(&st)
	if st != "queued" {
		t.Fatalf("job is %s", st)
	}
}
