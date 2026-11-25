package sources

import (
	"context"
	"strings"
	"testing"
)

// The list of a title's Releases never reads the whole releases table: every
// step on it goes through the releases_job index. Read per Slot instead, a
// show with 200 Slots in a Library of 300,000 Releases took minutes.
func TestLatestSearchJobsReadsReleasesByIndex(t *testing.T) {
	st, _ := openStore(t)
	for _, filter := range []string{"", " AND s.id = 1", " AND json_extract(i.numbers, '$.season') = 1"} {
		rows, err := st.Read().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+latestSearchJobsSQL(filter), 1, "a", "b", "c")
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		onReleases := 0
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
			if strings.Contains(detail, " r ") || strings.HasSuffix(detail, " r") {
				onReleases++
				if !strings.Contains(detail, "COVERING INDEX releases_job") {
					t.Errorf("filter %q: a step reads the releases table itself: %s", filter, detail)
				}
			}
		}
		_ = rows.Close()
		if onReleases != 3 {
			t.Errorf("filter %q: %d steps on releases, want one for each of the 3 parts:\n%s", filter, onReleases, strings.Join(plan, "\n"))
		}
	}
}
