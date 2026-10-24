package musicscore

import (
	"fmt"
	"testing"
)

// tplRow is a template-engine row: 8 items by distinct artists.
func tplRow(id, fam string, score float64) Row {
	r := Row{ID: id, Family: fam, Prior: PriorMixed, ChartSet: true, Min: 8}
	for k := 0; k < 8; k++ {
		r.Items = append(r.Items, Item{Key: fmt.Sprintf("%s-%d", id, k), Artist: fmt.Sprintf("%s-a%d", id, k), Score: score})
	}
	return r
}

func TestChartSetOverridesPriorFallback(t *testing.T) {
	// Three rows with the chart prior but ChartSet and Chart false must not
	// count as chart rows: all three may sit in the first 6 slots.
	var rows []Row
	for i := 0; i < 3; i++ {
		r := tplRow(fmt.Sprintf("new%d", i), fmt.Sprintf("n%d", i), 0.9-0.01*float64(i))
		r.Prior = PriorChart
		rows = append(rows, r)
	}
	p := NewPlan(rows, nil, nil, 1, "2026-10-24", nil)
	if got := p.All(nil); len(got) != 3 {
		t.Fatalf("rows = %d, want 3 (none counted as chart)", len(got))
	}
	// Three explicit chart rows: only 2 fit in any 6 slots.
	rows = nil
	for i := 0; i < 3; i++ {
		r := tplRow(fmt.Sprintf("ch%d", i), fmt.Sprintf("c%d", i), 0.9)
		r.Chart = true
		rows = append(rows, r)
	}
	p = NewPlan(rows, nil, nil, 1, "2026-10-24", nil)
	if got := p.All(nil); len(got) != 2 {
		t.Fatalf("chart rows = %d, want 2", len(got))
	}
}

func TestScoreFnSeesOnlyUnseenItems(t *testing.T) {
	var seen []int
	r := tplRow("a", "fa", 0.5)
	r.ScoreFn = func(unseen []Item) float64 { seen = append(seen, len(unseen)); return float64(len(unseen)) }
	// b shares 4 of a's items and goes first (head); a's score is then
	// computed over its 4 unseen items, under the gate of 8: zero.
	b := tplRow("b", "fb", 0.9)
	copy(b.Items[:4], r.Items[:4])
	p := NewPlan([]Row{r, b}, []string{"b"}, nil, 1, "2026-10-24", nil)
	p.All(nil)
	if len(seen) == 0 {
		t.Fatal("ScoreFn never called")
	}
}

func TestMinArtistsGate(t *testing.T) {
	r := tplRow("a", "fa", 0.5)
	r.MinArtists = 5
	for k := range r.Items {
		r.Items[k].Artist = fmt.Sprintf("art%d", k%4) // 4 artists, 2 items each
	}
	p := NewPlan([]Row{r}, nil, nil, 1, "2026-10-24", nil)
	if got := p.All(nil); len(got) != 0 {
		t.Fatalf("row with 4 artists served under MinArtists 5")
	}
	r.MinArtists = 4
	p = NewPlan([]Row{r}, nil, nil, 1, "2026-10-24", nil)
	if got := p.All(nil); len(got) != 1 {
		t.Fatal("row with 4 artists refused under MinArtists 4")
	}
}

func TestShapeRuleBringsACircleEvery8(t *testing.T) {
	var rows []Row
	for i := 0; i < 12; i++ {
		rows = append(rows, tplRow(fmt.Sprintf("sq%d", i), fmt.Sprintf("s%d", i), 0.9-0.01*float64(i)))
	}
	c := tplRow("circle", "artists", 0.1)
	c.Shape = "round"
	rows = append(rows, c)
	p := NewPlan(rows, nil, nil, 1, "2026-10-24", nil)
	got := p.All(nil)
	pos := -1
	for i, r := range got {
		if r.ID == "circle" {
			pos = i
		}
	}
	if pos < 0 || pos > 7 {
		t.Fatalf("circle row at %d, want within the first 8", pos)
	}
}
