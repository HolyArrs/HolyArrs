package mediaservers

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A successful call after a failure clears the error and stamps last_ok_at.
func TestRecordOKClearsError(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	m, err := Add(ctx, w.s, w.v, Input{Name: "p", Kind: "plex", URL: "http://10.0.0.9:32400", Token: "tk"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	markError(ctx, w.s, m.ID, "x")
	if got, _ := getServer(ctx, w.s.Read(), m.ID); got.LastError != "x" {
		t.Fatalf("error not stored: %+v", got)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	l.record(ctx, m, []Update{{}}, nil)
	got, err := getServer(ctx, w.s.Read(), m.ID)
	if err != nil || got.LastError != "" || got.LastOKAt == nil {
		t.Fatalf("%v %+v", err, got)
	}
}
