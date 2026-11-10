package requests

import (
	"context"
	"strings"
	"testing"
)

// The ready check for followed movies reads the follows first, never every
// movie and every slot: on a large Library that order took most of a minute
// with nobody following anything.
func TestTellMovieFollowsReadsFollowsFirst(t *testing.T) {
	r := newRig(t)
	rows, err := r.st.Read().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+tellMovieFollowsSQL)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if len(plan) == 0 || !strings.Contains(plan[0], " f ") && !strings.HasSuffix(plan[0], " f") {
		t.Fatalf("the plan does not start from the follows:\n%s", strings.Join(plan, "\n"))
	}
	for _, step := range plan {
		if strings.Contains(step, "slots_state") {
			t.Fatalf("the plan walks every slot by state:\n%s", strings.Join(plan, "\n"))
		}
	}
}
