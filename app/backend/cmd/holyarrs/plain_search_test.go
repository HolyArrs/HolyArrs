package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

// Search in plain words: the words are understood, the Library is filtered
// by kind, genre, year and about-words, Kid limits and parts that are off
// still hide titles.
//
// route: GET /api/v1/search
func TestPlainSearch(t *testing.T) {
	t.Parallel()
	setUp := func(k *kidRig) {
		t.Helper()
		err := k.st.Write(context.Background(), func(tx *sql.Tx) error {
			for _, x := range []struct {
				name, genres, overview string
				year                   int
			}{
				{"Gentle Meadow", `["Science Fiction"]`, "A small robot finds a home in space.", 2014},
				{"Iron Verdict", `["Science Fiction","Drama"]`, "A robot stands trial.", 2014},
				{"Paper Kite", `["Science Fiction"]`, "Two kids fly a kite.", 2014},
				{"Night Courier", `["Thriller"]`, "A courier at night.", 2014},
				{"Cold Precinct", `["Sci-Fi & Fantasy"]`, "Police on a cold planet.", 2014},
			} {
				if _, err := tx.Exec("UPDATE titles SET genres = ?, overview = ?, year = ? WHERE id = ?", x.genres, x.overview, x.year, k.ids[x.name]); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	type answer struct {
		Query struct {
			Plain    bool     `json:"plain"`
			Kind     string   `json:"kind"`
			YearFrom int      `json:"year_from"`
			Genres   []string `json:"genres"`
			Words    []string `json:"words"`
		} `json:"query"`
		Titles []struct {
			Name string `json:"name"`
		} `json:"titles"`
		More []json.RawMessage `json:"more"`
	}
	ask := func(s *signedIn, q string) (answer, []string) {
		t.Helper()
		var a answer
		if err := json.Unmarshal(s.call("GET", "/api/v1/search?q="+q, "", 200), &a); err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, x := range a.Titles {
			names = append(names, x.Name)
		}
		return a, names
	}

	k := newKidRig(t)
	setUp(k)
	a, names := ask(k.owner, "that%20space%20movie%20with%20the%20robot%20from%202014")
	if a.Query.Plain || a.Query.Kind != "movie" || a.Query.YearFrom != 2014 || !slices.Equal(a.Query.Genres, []string{"Science Fiction"}) || !slices.Equal(a.Query.Words, []string{"robot"}) {
		t.Fatalf("understood as %+v", a.Query)
	}
	if len(names) != 3 || names[2] != "Paper Kite" {
		t.Fatalf("robot titles first, then other space movies: %q", names)
	}
	if a.More == nil {
		t.Fatal("more is null")
	}
	sameNames(t, "kid", func() []string { _, n := ask(k.kai, "space%20movie%20with%20the%20robot%20from%202014"); return n }(), "Gentle Meadow", "Paper Kite")
	_, names = ask(k.owner, "space%20movie%20from%201999")
	sameNames(t, "wrong year", names)
	_, names = ask(k.owner, "space%20shows")
	sameNames(t, "show genre", names, "Cold Precinct")
	a, names = ask(k.owner, "gentle")
	if !a.Query.Plain {
		t.Fatalf("one word is a plain name: %+v", a.Query)
	}
	sameNames(t, "plain name", names, "Gentle Meadow")
	ask(k.owner, "")
	ask(k.owner, "%27%22%2A%29%28")

	off := newKidRig(t, parts.Movies)
	setUp(off)
	_, names = ask(off.owner, "space%20movies%20from%202014")
	sameNames(t, "movies off", names)
	_, names = ask(off.owner, "space%20from%202014")
	sameNames(t, "only the show is left", names, "Cold Precinct")
}
