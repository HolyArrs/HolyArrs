package wikidata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRevisionsMarksChangedEntitiesDue(t *testing.T) {
	rev := map[string]int{"Q1": 10, "Q2": 20}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("props") != "info" {
			t.Errorf("unexpected call %s", r.URL.RawQuery)
		}
		var parts []string
		for _, q := range strings.Split(r.URL.Query().Get("ids"), "|") {
			parts = append(parts, fmt.Sprintf(`"%s":{"lastrevid":%d}`, q, rev[q]))
		}
		fmt.Fprintf(w, `{"entities":{%s}}`, strings.Join(parts, ","))
	}))
	defer srv.Close()
	s, err := Open(filepath.Join(t.TempDir(), "w.sqlite"), Options{APIURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	s.db.Exec(`INSERT INTO answers VALUES ('movie:1', '{"qid":"Q1"}', 100), ('person:2', '{"qid":"Q2"}', 100), ('movie:3', NULL, 100)`)
	if n, err := s.CheckRevisions(ctx); err != nil || n != 0 || calls != 1 {
		t.Fatalf("baseline %d %v calls %d", n, err, calls)
	}
	rev["Q2"] = 21
	if n, err := s.CheckRevisions(ctx); err != nil || n != 1 {
		t.Fatalf("changed %d %v", n, err)
	}
	var at int64
	s.db.QueryRow(`SELECT fetched_at FROM answers WHERE k='person:2'`).Scan(&at)
	var other int64
	s.db.QueryRow(`SELECT fetched_at FROM answers WHERE k='movie:1'`).Scan(&other)
	if at != 0 || other != 100 {
		t.Fatalf("due %d other %d", at, other)
	}
}
