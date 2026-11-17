package migrate

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bigLidarr is a Lidarr read with many artists, albums and files.
func bigLidarr(n int) *Snapshot {
	s := &Snapshot{Kind: Lidarr, Via: ViaAPI, Version: "2.9.6",
		Folders:         []Folder{{Path: "/music"}},
		QualityProfiles: []Profile{{ID: 1, Name: "Lossless", Cutoff: "FLAC"}},
		MetadataProfiles: []Named{{ID: 1, Name: "Standard", Settings: map[string]any{
			"primaryalbumtypes":   []any{map[string]any{"name": "Album", "allowed": true}, map[string]any{"name": "EP", "allowed": true}},
			"secondaryalbumtypes": []any{map[string]any{"name": "Studio", "allowed": true}, map[string]any{"name": "Mixtape/Street", "allowed": true}},
			"releasestatuses":     []any{map[string]any{"name": "Official", "allowed": true}},
		}}},
	}
	for i := 0; i < n; i++ {
		ids := map[string]string{"musicbrainz": fmt.Sprintf("%08x-d505-4dad-8ec1-d04f521cbb56", i+1)}
		for j := 0; j < 12; j++ {
			ids[fmt.Sprintf("extra%02d", j)] = fmt.Sprintf("%d-%d", i, j)
		}
		t := Title{SourceID: int64(i + 1), Kind: "artist", Name: fmt.Sprintf("Artist %d", i), Path: fmt.Sprintf("/music/Artist %d", i),
			Monitored: i%3 != 0, ProfileID: 1, IDs: ids, Tags: []string{"a"},
			Extra: map[string]any{"new_albums": "all", "metadata_profile_id": int64(1), "artist_metadata_id": int64(i + 10)}}
		for a := 0; a < 5; a++ {
			t.MusicAlbums = append(t.MusicAlbums, MusicAlbum{MBID: fmt.Sprintf("%08x-%04x-4dad-8ec1-d04f521cbb56", i+1, a), Monitored: a%2 == 0})
			t.Files = append(t.Files, TitleFile{Name: fmt.Sprintf("%02d - Track.flac", a), Quality: "FLAC"})
		}
		s.Titles = append(s.Titles, t)
	}
	return s
}

func TestLargeLidarrMoveStartsFromItsPreview(t *testing.T) {
	l := newMoveLab(t)
	music := filepath.ToSlash(t.TempDir())
	s := bigLidarr(400)
	s.finish()
	saved, err := Save(context.Background(), l.st, 1, s)
	if err != nil {
		t.Fatal(err)
	}
	l.snaps = append(l.snaps, saved.ID)
	ch := Choices{PathRules: append(append([]PathRule{}, l.rules...), PathRule{From: "/music", To: music}), People: l.people}
	want := PlanHash(l.planWith(t, ch))
	for i := 0; i < 20; i++ {
		if got := PlanHash(l.planWith(t, ch)); got != want {
			t.Fatalf("the same read and choices gave another plan (run %d)", i)
		}
	}
	mv := l.move(t, ch)
	for _, n := range mv.Report.Next {
		if strings.Contains(n, "could not start") {
			t.Fatalf("the move refused its own preview: %s", n)
		}
	}
}

func (l *moveLab) planWith(t *testing.T, ch Choices) *Plan {
	t.Helper()
	p, err := LoadPlan(context.Background(), l.st, l.snaps, ch)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanHashCoversOnlyWhatTheMoveDoes(t *testing.T) {
	s := bigLidarr(3)
	ch := Choices{PathRules: []PathRule{{From: "/music", To: "/m"}}}
	p := BuildPlan([]Input{{ID: 1, Snap: s}}, ch)
	want := PlanHash(p)
	// Wording, positions and the not-moved list do not refuse a move.
	p.NotMoved = append(p.NotMoved, NotMoved{What: "x"})
	p.Notes = append(p.Notes, "note")
	for gi := range p.Groups {
		for ii := range p.Groups[gi].Items {
			p.Groups[gi].Items[ii].Detail += " (reworded)"
			p.Groups[gi].Items[ii].From = "lidarr titles[99]"
		}
	}
	if PlanHash(p) != want {
		t.Fatal("a harmless difference changed the approval")
	}
	// What an item does, the choices and the read still count.
	other := bigLidarr(3)
	other.Titles[0].Monitored = !other.Titles[0].Monitored
	for name, q := range map[string]*Plan{
		"item changed":  BuildPlan([]Input{{ID: 1, Snap: other}}, ch),
		"rules changed": BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{PathRules: []PathRule{{From: "/music", To: "/n"}}}),
		"group skipped": BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{PathRules: ch.PathRules, Skip: []string{GroupTitles}}),
		"another read":  BuildPlan([]Input{{ID: 2, Snap: s}}, ch),
	} {
		if PlanHash(q) == want {
			t.Errorf("%s: the approval did not change", name)
		}
	}
}

func TestReportBeforeStartHasTimeAndSummary(t *testing.T) {
	when := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	md := (&Report{Move: 3, At: when, Lines: []Line{}, Counts: map[string]int{}, Next: []string{"x"}}).Markdown()
	if !strings.Contains(md, "- When: 2026-11-10 12:00 UTC") || !strings.Contains(md, "- In short: nothing was moved") {
		t.Fatalf("report:\n%s", md)
	}
	if strings.Contains((&Report{Counts: map[string]int{}}).Markdown(), "0001-01-01") {
		t.Fatal("a report with no time printed year 1")
	}
}
