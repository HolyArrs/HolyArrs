package suggest

import (
	"fmt"
	"testing"
	"time"
)

func momentRow(id, genre string, base int) Row {
	items := make([]Item, 20)
	for i := range items {
		items[i] = Item{TMDBID: fmt.Sprint(base + i), Kind: "movie", Name: fmt.Sprint(id, i), Genres: []string{genre}, Released: "2001-01-01", Pop: 10}
	}
	return Row{ID: TypePopularGenre + ":" + genre, Type: TypePopularGenre, Genre: genre, Title: genre, Items: items, Score: 0.5}
}

func tailPos(pg Page, id string) int {
	for i, r := range append(append([]Row{}, pg.Head...), pg.Tail...) {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// TestAssembleAt_MomentLiftsFittingTailRow: the time-of-day lift reaches the
// row order through AssembleAt, and kids get none.
func TestAssembleAt_MomentLiftsFittingTailRow(t *testing.T) {
	late := Moment{Local: time.Date(2026, 10, 6, 23, 30, 0, 0, time.UTC)}
	thr, fam := momentRow("thr", "Thriller", 1000), momentRow("fam", "Family", 2000)
	flipped := false
	for member := int64(1); member <= 200; member++ {
		p := Pool{Seeds: 4, Date: "2026-10-06", Rows: []Row{fam, thr}}
		plain, lit := AssembleAt(p, member, Moment{}), AssembleAt(p, member, late)
		pp, lp := tailPos(plain, thr.ID), tailPos(lit, thr.ID)
		if pp < 0 || lp < 0 {
			t.Fatalf("member %d: thriller row missing from page (%d, %d)", member, pp, lp)
		}
		if lp > pp {
			t.Fatalf("member %d: late night moved the thriller row down (%d -> %d)", member, pp, lp)
		}
		if lp < pp {
			flipped = true
		}
		p.Kid = true
		kp, kl := AssembleAt(p, member, Moment{}), AssembleAt(p, member, late)
		if fmt.Sprint(order(kp.Head), order(kp.Tail)) != fmt.Sprint(order(kl.Head), order(kl.Tail)) {
			t.Fatalf("member %d: kid page changed with the moment: %v vs %v", member, order(kp.Head), order(kl.Head))
		}
	}
	if !flipped {
		t.Fatal("late night never lifted the thriller row for any member")
	}
}

// TestMergeTop_DedupesSortsCaps keeps each title once, sorts by Pop with ties
// in first-seen order and caps at n.
func TestMergeTop_DedupesSortsCaps(t *testing.T) {
	a := []Item{{Kind: "movie", TMDBID: "1", Pop: 5}, {Kind: "movie", TMDBID: "2", Pop: 9}}
	b := []Item{{Kind: "movie", TMDBID: "1", Pop: 5}, {Kind: "show", TMDBID: "1", Pop: 7}}
	got := MergeTop([][]Item{a, b}, 2)
	if len(got) != 2 || got[0].TMDBID != "2" || got[1].Kind != "show" {
		t.Fatalf("got %+v", got)
	}
	if len(MergeTop(nil, 5)) != 0 {
		t.Fatal("nil lists gave items")
	}
	tie := MergeTop([][]Item{{{Kind: "movie", TMDBID: "x", Pop: 1}, {Kind: "movie", TMDBID: "y", Pop: 1}}, {{Kind: "movie", TMDBID: "z", Pop: 1}}}, 5)
	if len(tie) != 3 || tie[0].TMDBID != "x" || tie[1].TMDBID != "y" || tie[2].TMDBID != "z" {
		t.Fatalf("ties reordered: %+v", tie)
	}
}

// TestSortByScore_DescStable sorts high to low and keeps ties in place.
func TestSortByScore_DescStable(t *testing.T) {
	items := []Item{{TMDBID: "a"}, {TMDBID: "b"}, {TMDBID: "c"}}
	s := map[string]float64{"a": 1, "b": 3, "c": 1}
	SortByScore(items, func(it Item) float64 { return s[it.TMDBID] })
	if items[0].TMDBID != "b" || items[1].TMDBID != "a" || items[2].TMDBID != "c" {
		t.Fatalf("order %v %v %v", items[0].TMDBID, items[1].TMDBID, items[2].TMDBID)
	}
}
