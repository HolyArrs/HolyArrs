package mediafiles

import (
	"context"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The feed check and its probe list the looking Titles on every run. The
// wanted Slots must be read once for all Titles, not once per Title: per
// Title it is Titles x wanted Slots, about 30 s on a library with 2,800
// Titles and 37,000 wanted Slots.
func TestTitlesLookingReadsSlotsOnce(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	rows, err := s.Read().QueryContext(context.Background(), "EXPLAIN QUERY PLAN SELECT * FROM titles WHERE "+titlesLookingWhere, "wanted", "better_wanted", "held")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	plan := strings.Join(details, "; ")
	if !strings.Contains(plan, "LIST SUBQUERY") || strings.Contains(plan, "SCAN titles") {
		t.Fatalf("looking Titles read the Slots per Title: %s", plan)
	}
}
