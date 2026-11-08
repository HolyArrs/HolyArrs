package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// Search's "In your Library" row: local matches by name, original name and
// year with accents and case folded; scopes; Kid limits.
//
// route: GET /api/v1/library/search
func TestLibrarySearch(t *testing.T) {
	t.Parallel()
	k := lbNewKidRig(t)
	err := k.st.Write(context.Background(), func(tx *sql.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{"UPDATE titles SET original_name = 'Prairie Douce', year = 2021, people = '[\"Zoë Ward\",\"Ann Lee\"]' WHERE id = ?", []any{k.ids["Gentle Meadow"]}},
			{"UPDATE titles SET people = '[\"Zoë Ward\"]' WHERE id = ?", []any{k.ids["Iron Verdict"]}},
			{`INSERT INTO parts(created_at, created_by, updated_at, updated_by, title_id, kind, name, source_ids) VALUES ('2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}', 9105, 'album', 'Static Ünder Glass', '{"musicbrainz":"AbC-1"}')`, nil},
		} {
			if _, err := tx.Exec(q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	type answer struct {
		Titles []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"titles"`
		Albums []mediafiles.LibraryAlbum  `json:"albums"`
		People []mediafiles.LibraryPerson `json:"people"`
	}
	ask := func(s *lbSignedIn, q string) answer {
		t.Helper()
		var a answer
		if err := json.Unmarshal(s.call("GET", "/api/v1/library/search?"+q, "", 200), &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	names := func(a answer) []string {
		out := []string{}
		for _, x := range a.Titles {
			out = append(out, x.Name)
		}
		return out
	}
	start := time.Now()
	lbSameNames(t, "name, folded", names(ask(k.owner, "q=GENTLE")), "Gentle Meadow")
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("library search took %v", d)
	}
	lbSameNames(t, "original name", names(ask(k.owner, "q=prairie")), "Gentle Meadow")
	lbSameNames(t, "year", names(ask(k.owner, "q=gentle%202021")), "Gentle Meadow")
	lbSameNames(t, "wrong year", names(ask(k.owner, "q=gentle%201999")))
	lbSameNames(t, "movies scope", names(ask(k.owner, "q=static&kind=movie")))
	lbSameNames(t, "music scope", names(ask(k.owner, "q=static&kind=music")), "Loud Static")
	if a := ask(k.owner, "q=under&kind=music"); len(a.Albums) != 1 || a.Albums[0].Artist != "Loud Static" {
		t.Fatalf("album by folded name: %+v", a.Albums)
	} else if a.Albums[0].Image != music.ArtPrefix+"album/abc-1" {
		t.Fatalf("album cover route %q", a.Albums[0].Image)
	}
	if a := ask(k.owner, "q=under&kind=movie"); len(a.Albums) != 0 {
		t.Fatalf("albums in the movies scope: %+v", a.Albums)
	}
	a := ask(k.owner, "q=zoe&kind=people")
	if len(a.Titles) != 0 || len(a.People) != 1 || a.People[0].Name != "Zoë Ward" || len(a.People[0].Titles) != 2 {
		t.Fatalf("people scope: %+v", a)
	}
	// A Kid limited to PG never sees titles above the limit, nor their people.
	lbSameNames(t, "kid", names(ask(k.kai, "q=iron")))
	kid := ask(k.kai, "q=zoe&kind=people")
	if len(kid.People) != 1 || len(kid.People[0].Titles) != 1 || kid.People[0].Titles[0].Name != "Gentle Meadow" {
		t.Fatalf("kid people: %+v", kid.People)
	}
	if body := string(k.kai.call("GET", "/api/v1/library/search?q=zoe", "", 200)); lbMentionsBlocked(body) != "" {
		t.Fatalf("kid sees %q: %s", lbMentionsBlocked(body), body)
	}
	k.owner.call("GET", "/api/v1/library/search?q=x&kind=nope", "", 400)
}
