package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
)

// The title list carries a title's score and vote count, and leaves both
// out when the title has none.
func TestTitleListScore(t *testing.T) {
	t.Parallel()
	srv, s, _ := libSetup(t)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, t.TempDir()), 201)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	list := func(set string) map[string]json.RawMessage {
		t.Helper()
		if err := s.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE titles SET " + set + " WHERE id = 1")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		var out struct {
			Titles []map[string]json.RawMessage `json:"titles"`
		}
		if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles", "", 200), &out); err != nil || len(out.Titles) != 1 {
			t.Fatalf("list %v %v", out, err)
		}
		return out.Titles[0]
	}
	if v := list("score = 7.4, votes = 900"); string(v["score"]) != "7.4" || string(v["votes"]) != "900" {
		t.Fatalf("score %s votes %s", v["score"], v["votes"])
	}
	for _, set := range []string{"score = NULL, votes = 0", "score = 0, votes = 0"} {
		if v := list(set); v["score"] != nil || v["votes"] != nil {
			t.Fatalf("%s: score %s votes %s", set, v["score"], v["votes"])
		}
	}
}
