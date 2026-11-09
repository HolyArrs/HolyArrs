package alerts

import (
	"strings"
	"testing"
)

func TestTrackWordsFoldsAndLabels(t *testing.T) {
	one := []Track{{Album: "Red", Year: 2012, Disc: 1, Number: 3, Name: "A", Discs: 1}}
	if got := trackWords(one); got != "Red (2012) track 03" {
		t.Fatalf("one track %q", got)
	}
	same := []Track{{Album: "Red", Number: 1, Name: "A"}, {Album: "Red", Number: 2, Name: "B"}}
	if got := trackWords(same); got != "Red" {
		t.Fatalf("one album %q", got)
	}
	mixed := []Track{{Album: "Red", Number: 1}, {Album: "Blue", Number: 1}, {Album: "Blue", Number: 2}, {Album: "Gold", Number: 4}}
	if got := trackWords(mixed); got != "4 tracks" {
		t.Fatalf("mixed %q", got)
	}
	if trackWords(nil) != "" {
		t.Fatal("empty list named")
	}
	lines := strings.Split(trackLines(mixed), "\n")
	if len(lines) != 4 || lines[3] != "Gold: track 04" {
		t.Fatalf("lines %q", lines)
	}
	if got := trackLines(same); got != "track 01 A\ntrack 02 B" {
		t.Fatalf("same-album lines %q", got)
	}
	if got := (Track{Disc: 2, Number: 1, Discs: 2}).Label(); got != "disc 2 track 01" {
		t.Fatalf("multi-disc label %q", got)
	}
	if got := (Track{Name: "X", Number: 5}).Label(); got != "track 05" {
		t.Fatalf("no-album label %q", got)
	}
}
