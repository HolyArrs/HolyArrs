package library

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Two movies with the same name and year both fit the folder: the folder is Unsure,
// and its reason names both instead of "best match ... at 100%", which reads as if the name were the doubt.
func TestPlanAdoptTwoTitlesFitEqually(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	writeFile(t, filepath.Join(lf.Path, "Paper Harbour (2023)", "ph.mkv"), "p")
	twins := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "7001"}},
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "7002"}},
	}}
	plan, err := PlanAdopt(context.Background(), s.Read(), twins, lf.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := byFolder(plan)["Paper Harbour (2023)"]
	if c.Status != "unsure" || c.Match == nil || c.Match.Score < MatchSure {
		t.Fatalf("status %s match %+v (%s)", c.Status, c.Match, c.Why)
	}
	if strings.Contains(c.Why, "%") || !strings.Contains(c.Why, "both fit this folder name") || !strings.Contains(c.Why, "2023") {
		t.Fatalf("why %q", c.Why)
	}
}
