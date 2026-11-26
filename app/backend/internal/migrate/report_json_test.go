package migrate

import (
	"encoding/json"
	"strings"
	"testing"
)

// A downloader-only import has no apps; its report must still send lists, not null.
func TestReportSendsEmptyListsNotNull(t *testing.T) {
	for _, r := range []any{Report{}, &Report{Move: 1}} {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "null") {
			t.Fatalf("report has null: %s", b)
		}
		for _, k := range []string{`"apps":[]`, `"lines":[]`, `"not_moved":[]`, `"needs_input":[]`, `"next":[]`, `"counts":{}`} {
			if !strings.Contains(string(b), k) {
				t.Fatalf("missing %s in %s", k, b)
			}
		}
	}
}
