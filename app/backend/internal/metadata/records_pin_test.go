package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// JSON golden of the records metadata returns: the lab movie in full, and
// every other record type as its zero value, which lists every field name
// and tag. Tags and field order must not change when the package moves.
func TestPinRecordsGolden(t *testing.T) {
	out := map[string]any{
		"lab_movie":     Lab().Movies[0],
		"movie":         Movie{},
		"collection":    MovieCollection{},
		"show":          Show{},
		"season":        Season{},
		"episode":       Episode{},
		"person":        Person{},
		"person_page":   PersonPage{},
		"person_movie":  PersonMovieCredit{},
		"movie_credit":  MoviePersonCredit{},
		"review":        Review{},
		"review_page":   ReviewPage{},
		"person_movies": PersonMoviePage{},
	}
	got, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "pin_records.golden.json")
	if os.Getenv("PIN_UPDATE") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
		t.Fatalf("records changed:\n%s", got)
	}
}
