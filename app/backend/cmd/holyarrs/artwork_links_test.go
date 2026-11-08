package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// A cached poster is stored as "poster:<cache name>"; the title list and the
// title page must give the web app a link it can load, or every movie shows
// its initials instead of its poster.
func TestTitleArtworkIsALink(t *testing.T) {
	t.Parallel()
	srv, s, _ := libSetup(t)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, t.TempDir()), 201)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET artwork = '["poster:63dc68cd95ad59bdbd753f3209d71db6.jpg"]' WHERE id = 1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := `"artwork":["/api/v1/artwork/63dc68cd95ad59bdbd753f3209d71db6.jpg"]`
	for _, u := range []string{"/api/v1/titles/1", "/api/v1/titles"} {
		if b := string(do(t, "GET", srv.URL+u, "", 200)); !strings.Contains(b, want) {
			t.Fatalf("%s: artwork is not a link: %s", u, b)
		}
	}
}
