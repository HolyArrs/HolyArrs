package downloads

import (
	"context"
	"testing"
)

// A Downloader with a path pair that places links keeps its finished jobs
// and their files: the Library entries point at them.
func TestClearFinishedKeepsLinkedDownloads(t *testing.T) {
	cases := []struct {
		name    string
		pathMap string
		cleared bool
	}{
		{"no pairs", `[]`, true},
		{"pair without links", `[{"theirs":"/remote","ours":"/data"}]`, true},
		{"pair with links", `[{"theirs":"/remote","ours":"/data","link":true}]`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, m, id, spy := usenetDownload(t)
			if err := execSQL(w, "UPDATE downloaders SET path_map = '"+tc.pathMap+"'"); err != nil {
				t.Fatal(err)
			}
			m.ClearFinished(context.Background(), id)
			if got := len(spy.removed) == 1; got != tc.cleared {
				t.Fatalf("cleared = %v, want %v (%v)", got, tc.cleared, spy.removed)
			}
		})
	}
}
