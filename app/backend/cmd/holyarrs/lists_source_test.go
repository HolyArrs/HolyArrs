package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A movie a list adds says in its title-added History entry which list it came
// from, by the list's name.
func TestListAddedTitleNamesItsList(t *testing.T) {
	t.Parallel()
	srv, s := listSetup(t)
	movies := filepath.Join(t.TempDir(), "movies")
	_ = os.MkdirAll(movies, 0o755)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, movies), 201)
	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"Lab list","kind":"tmdb_list","ref":"7"}`, 201)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var raw string
		err := s.ReadEvents().QueryRowContext(context.Background(), "SELECT data FROM events WHERE kind = 'title_added' ORDER BY id DESC LIMIT 1").Scan(&raw)
		if err == nil {
			var d map[string]any
			_ = json.Unmarshal([]byte(raw), &d)
			if d["why_added"] != "list" || d["list_name"] != "Lab list" {
				t.Fatalf("title_added data %s", raw)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no title_added event: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
