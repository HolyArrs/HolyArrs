package musicscore

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// Exact values for each Phase 3 formula on fixed inputs (plan §6 Phase 3).
func TestPhase3Formulas(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"new from / spotlight 0.6A+0.4F14", ScoreNewFrom(0.5, 0.25), 0.4},
		{"fresh songs 0.7P+0.3F14", ScoreFreshSong(0.8, 0.5), 0.71},
		{"missing 0.5A+0.3P+0.2latest", ScoreMissing(0.6, 0.2, 1), 0.56},
		{"coming soon 0.7A+0.3P", ScoreComingSoon(0.5, 1), 0.65},
		{"since you have 0.5S+0.3P+0.2G", ScoreSinceYouHave(0.9, 0.1, 0.5), 0.58},
		{"genre shelf 0.5P+0.3G+0.2M", ScoreGenreShelf(0.4, 1, 0), 0.5},
		{"guest 0.6A+0.4P", ScoreGuest(0.5, 0.25), 0.4},
		{"near taste capped", ScoreNearTaste(0.9, 1), 1},
		{"near taste", ScoreNearTaste(0.5, 0.25), 0.55},
		{"artist P charted 0.6Pc+0.4L", ArtistPopularity(0.5, true, 0.5), 0.5},
		{"artist P uncharted min(0.3,0.3L)", ArtistPopularity(0.9, false, 0.5), 0.15},
		{"S relation 0.8", Similarity(0.2, true), 0.8},
		{"S max over sources", Similarity(0.95, true), 0.95},
		{"S listenbrainz only", Similarity(0.4, false), 0.4},
		{"F14 at 14 days", Freshness("2026-10-08", 14, mustDay("2026-10-22")), math.Exp(-1)},
		{"A follow only", Affinity(Acts{Follows: true}), 0.5},
	} {
		if !near(c.got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
		}
	}
	sh := Shares(map[string]int{"Alternative Rock": 3, "Indie": 1, "none": 0})
	if !near(sh["alternativerock"], 0.75) || !near(sh["indie"], 0.25) || len(sh) != 2 {
		t.Fatalf("shares %v", sh)
	}
	if len(Shares(map[string]int{"x": 0})) != 0 {
		t.Fatal("no votes → no shares")
	}
	g := map[string]float64{"hiphop": 0.5, "rap": 0.9, "pop": 0.2}
	if !near(ItemGenre(g, []string{"Hip-Hop/Rap"}), 0.9) || ItemGenre(g, nil) != 0 || !near(ItemGenre(g, []string{"hip hop"}), 0.5) {
		t.Fatal("G_item")
	}
}

func sc(key string, score, a, g float64, genre string) Scored {
	return Scored{Item: Item{Key: key, Score: score}, A: a, GItem: g, Genre: genre}
}

func keys(l []Scored) []string {
	var out []string
	for _, x := range l {
		out = append(out, x.Key)
	}
	return out
}

func TestRankAndDiversify(t *testing.T) {
	l := []Scored{sc("a", 0.9, 1, 0, "rock"), sc("b", 0.85, 1, 0, "rock"), sc("c", 0.8, 1, 0, "pop"), sc("d", 0.7, 1, 0, "")}
	Rank(l, 1, "2026-10-22")
	if !reflect.DeepEqual(keys(l), []string{"a", "b", "c", "d"}) {
		t.Fatal(keys(l))
	}
	// b pays 0.15 after a: 0.70 < c's 0.80, so c moves up.
	if got := keys(Diversify(l)); !reflect.DeepEqual(got, []string{"a", "c", "b", "d"}) {
		t.Fatalf("diversify %v", got)
	}
	// Items without a genre are never penalised.
	n := []Scored{sc("x", 0.9, 1, 0, ""), sc("y", 0.89, 1, 0, ""), sc("z", 0.8, 1, 0, "pop")}
	if got := keys(Diversify(n)); !reflect.DeepEqual(got, []string{"x", "y", "z"}) {
		t.Fatalf("no genre %v", got)
	}
	// Equal scores: the day's tie hash decides, the same all day.
	e1 := []Scored{sc("p", 0.5, 0, 0, ""), sc("q", 0.5, 0, 0, ""), sc("r", 0.5, 0, 0, "")}
	e2 := []Scored{sc("r", 0.5, 0, 0, ""), sc("p", 0.5, 0, 0, ""), sc("q", 0.5, 0, 0, "")}
	Rank(e1, 7, "2026-10-22")
	Rank(e2, 7, "2026-10-22")
	if !reflect.DeepEqual(keys(e1), keys(e2)) {
		t.Fatal("ties must not depend on input order")
	}
}

func row(n int) []Scored {
	var l []Scored
	for i := 0; i < n; i++ {
		l = append(l, sc(string(rune('a'+i)), 1-float64(i)/20, 0.5, 0.1, ""))
	}
	return l
}

func TestExplorationSlot(t *testing.T) {
	// No A=0 ∧ G≥0.3 candidate: the order equals the non-slot order.
	base := row(10)
	if !reflect.DeepEqual(keys(Explore(base)), keys(base)) {
		t.Fatal("no candidate must keep the order")
	}
	// A = 0 but G under 0.3 is no candidate either.
	l := row(10)
	l[7].A, l[7].GItem = 0, 0.29
	if !reflect.DeepEqual(keys(Explore(l)), keys(l)) {
		t.Fatal("G 0.29 is not enough")
	}
	// The best candidate goes to position 4.
	l = row(10)
	l[6].A, l[6].GItem = 0, 0.3
	l[8].A, l[8].GItem = 0, 0.9
	got := keys(Explore(l))
	if got[3] != "g" || len(got) != 10 || got[4] != "d" || got[7] != "h" {
		t.Fatalf("slot %v", got)
	}
	// A candidate already in the first four places: no move.
	l = row(10)
	l[1].A, l[1].GItem = 0, 0.5
	if !reflect.DeepEqual(keys(Explore(l)), keys(l)) {
		t.Fatal("candidate at place 2 must stay")
	}
	// Rows under 9 items get no slot.
	s := row(8)
	s[7].A, s[7].GItem = 0, 1
	if !reflect.DeepEqual(keys(Explore(s)), keys(s)) {
		t.Fatal("8 items: no slot")
	}
}

func mustDay(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}
