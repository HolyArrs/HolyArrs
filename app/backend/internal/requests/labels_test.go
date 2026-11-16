package requests

import (
	"context"
	"slices"
	"testing"
)

// The Plex label list holds asked-for titles in the Library with everyone
// who asked, earliest first, and only those available since the time given.
func TestAskedForLabels(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	first, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, first.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	list := AskedForLabels(r.st)
	if got, err := list(ctx, ""); err != nil || len(got) != 0 {
		t.Fatalf("before it is available: %+v %v", got, err)
	}
	if _, err := r.svc.Ask(ctx, r.bo, "693134"); err != nil {
		t.Fatal(err)
	}
	pinExec(t, r, "UPDATE asks SET state = 'available', available_at = '2026-11-10T00:00:00.000000000Z' WHERE id = ?", first.Ask.ID)
	got, err := list(ctx, "")
	if err != nil || len(got) != 1 || got[0].TitleID == 0 || !slices.Equal(got[0].Askers, []string{r.ana.Name, r.bo.Name}) {
		t.Fatalf("available: %+v %v", got, err)
	}
	// A deleted person's stored name (with its hidden mark) never becomes a label.
	pinExec(t, r, "UPDATE users SET name = ? WHERE name = ?", r.bo.Name+"\u2063"+"5", r.bo.Name)
	if got, err := list(ctx, ""); err != nil || len(got) != 1 || !slices.Equal(got[0].Askers, []string{r.ana.Name}) {
		t.Fatalf("deleted asker: %+v %v", got, err)
	}
	if got, err := list(ctx, "2026-11-11T00:00:00.000000000Z"); err != nil || len(got) != 0 {
		t.Fatalf("since after: %+v %v", got, err)
	}
	if got, err := list(ctx, "2026-11-09T00:00:00.000000000Z"); err != nil || len(got) != 1 {
		t.Fatalf("since before: %+v %v", got, err)
	}
}
