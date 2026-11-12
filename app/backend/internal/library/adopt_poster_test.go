package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A match carries the poster the lookup already returned, so the
// Import list can show it without another provider call.
func TestPlanAdoptMatchCarriesPoster(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	if err := os.MkdirAll(filepath.Join(lf.Path, "Heat (1995)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lf.Path, "Heat (1995)", "Heat.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := &metadata.Static{Movies: []metadata.Movie{{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949"}, Poster: "https://image.tmdb.org/t/p/w500/h.jpg"}}}
	plan, err := PlanAdopt(context.Background(), s.Read(), md, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Folders) != 1 || plan.Folders[0].Match == nil || plan.Folders[0].Match.Poster != "https://image.tmdb.org/t/p/w500/h.jpg" {
		t.Fatalf("plan %+v", plan.Folders)
	}
}
