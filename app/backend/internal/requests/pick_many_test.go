package requests

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Picking more than one quality is off until the owner turns it on, and it
// saves and reads back.
func TestPickManyDefaultsOffAndSaves(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	s, err := ReadSettings(ctx, r.st.Read())
	if err != nil || s.PickMany {
		t.Fatalf("default %+v %v", s, err)
	}
	s.PickMany = true
	if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	if s, _ = ReadSettings(ctx, r.st.Read()); !s.PickMany {
		t.Fatal("pick many did not save")
	}
	s.PickMany = false
	if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	if s, _ = ReadSettings(ctx, r.st.Read()); s.PickMany {
		t.Fatal("pick many did not turn off")
	}
}
