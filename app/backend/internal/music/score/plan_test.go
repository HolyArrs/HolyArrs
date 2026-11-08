package musicscore

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

var planMax = map[string]int{"mix": 6, "because": 4, "shelf": 3, "a": 1, "b": 1, "c": 1, "d": 2, "e": 2, "f": 2, "g": 2, "h": 2}

func TestPlan_VarietyAndCadence(t *testing.T) {
	rows := planRows(map[string]int{"mix": 6, "because": 4, "shelf": 3, "a": 1, "b": 1, "c": 1, "d": 2, "e": 2, "f": 2, "g": 2, "h": 2}, map[string]bool{"mix": true})
	p := NewPlan(rows, nil, planMax, 1, "2026-10-23", nil)
	got := p.All(nil)
	if len(got) < 20 {
		t.Fatalf("rows = %d", len(got))
	}
	count := map[string]int{}
	for i, r := range got {
		count[r.Family]++
		if i > 0 && got[i-1].Family == r.Family {
			t.Fatalf("adjacent %s at %d", r.Family, i)
		}
		slot := i + 1 + planOffset
		if slot >= 8 && slot%4 == 0 && slot <= 20 && !r.Disc {
			t.Fatalf("slot %d is %s, want a discovery row", slot, r.ID)
		}
	}
	for f, n := range count {
		if n > planMax[f] {
			t.Fatalf("%s over its max: %d", f, n)
		}
	}
}

func TestPlan_ChartWindow(t *testing.T) {
	rows := planRows(map[string]int{"a": 1, "b": 1, "c": 1, "d": 2, "e": 2, "f": 2}, nil)
	for i := range rows {
		if rows[i].Family == "d" || rows[i].Family == "e" || rows[i].Family == "f" {
			rows[i].Prior = PriorChart
			for k := range rows[i].Items {
				rows[i].Items[k].Score = 2
			}
		}
	}
	got := NewPlan(rows, nil, planMax, 1, "2026-10-23", nil).All(nil)
	for i := range got {
		c := 0
		for _, r := range got[max(0, i-5) : i+1] {
			if r.Prior == PriorChart {
				c++
			}
		}
		if c > 2 {
			t.Fatalf("%d chart rows in 6 ending at %d", c, i)
		}
	}
}

func TestPlan_StableAndDedupAcrossBatches(t *testing.T) {
	rows := planRows(map[string]int{"mix": 6, "because": 4, "shelf": 3, "a": 1, "b": 1, "c": 1, "d": 2, "e": 2, "f": 2}, map[string]bool{"mix": true})
	for i := range rows {
		rows[i].Items[0].Album = "shared"
	}
	page := func() [][]Row {
		p := NewPlan(rows, nil, planMax, 1, "2026-10-23", nil)
		var out [][]Row
		for k := 0; ; k++ {
			r, more := p.Page(k, nil)
			out = append(out, r)
			if !more {
				break
			}
		}
		return out
	}
	a, b := page(), page()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same inputs, different pages")
	}
	if len(a) < 3 {
		t.Fatalf("pages = %d", len(a))
	}
	n := 0
	for _, pg := range a {
		for _, r := range pg {
			for _, it := range r.Items {
				if it.Album == "shared" {
					n++
				}
			}
		}
	}
	if n != 1 {
		t.Fatalf("shared album placed %d times", n)
	}
}

func TestPlan_ReserveAndRerank(t *testing.T) {
	rows := planRows(map[string]int{"a": 1, "b": 1, "c": 1, "d": 2, "e": 2, "f": 2, "g": 2, "h": 2}, nil)
	for i := range rows {
		if rows[i].ID == "b:0" {
			for k := range rows[i].Items {
				rows[i].Items[k].Key = fmt.Sprintf("a:0%d", k)
				rows[i].Items[k].Score = 0.99
			}
		}
	}
	p := NewPlan(rows, []string{"a:0", "b:0"}, planMax, 1, "2026-10-23", nil)
	first, _ := p.Page(0, nil)
	for _, r := range first {
		if r.ID == "b:0" {
			t.Fatal("b:0 served under its gate")
		}
	}
	if len(first) != PlanBatch {
		t.Fatalf("batch 1 = %d rows, want a reserve to fill", len(first))
	}
	before := append([]Row{}, first...)
	p.Page(1, nil)
	again, _ := p.Page(0, nil)
	if !reflect.DeepEqual(before, again) {
		t.Fatal("served rows moved after a later batch")
	}
}

func TestPlan_RerankUnserved(t *testing.T) {
	rows := planRows(map[string]int{"a": 1, "b": 1, "c": 1, "d": 2, "e": 2, "f": 2, "g": 2, "h": 2}, nil)
	for i := range rows {
		if rows[i].ID == "h:0" {
			for k := 0; k < 6; k++ {
				rows[i].Items[k].Key = fmt.Sprintf("a:0%d", k)
			}
			for k := range rows[i].Items {
				rows[i].Items[k].Score = 0.85
			}
		}
	}
	p := NewPlan(rows, []string{"a:0"}, planMax, 1, "2026-10-23", nil)
	pre := p.Planned()
	posH := func(l []string) int {
		for i, x := range l {
			if x == "h:0" {
				return i
			}
		}
		return -1
	}
	p.Page(0, nil)
	post := p.Planned()
	if posH(pre) < 0 || posH(pre) < PlanBatch && posH(post) >= 0 && posH(post) < PlanBatch {
		t.Skip("h:0 served in batch 1")
	}
	if posH(post) >= 0 && posH(post) <= posH(pre) {
		t.Fatalf("h:0 at %d before, %d after: want it lower once its items are placed", posH(pre), posH(post))
	}
}

func TestPlanStore_Versions(t *testing.T) {
	var s PlanStore
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	k := PlanKey{Member: 1, Date: "2026-10-23", View: "global"}
	for i := 0; i < 3; i++ {
		s.Put(k, &Plan{}, now)
	}
	if _, ok := s.Get(k, 1, now); ok {
		t.Fatal("version 1 kept past two")
	}
	if p, ok := s.Get(k, 3, now); !ok || p.Version != 3 {
		t.Fatal("version 3 missing")
	}
	s.MarkStale(-1)
	if _, ok := s.Current(k, now); ok {
		t.Fatal("stale plan is current")
	}
	if _, ok := s.Get(k, 3, now); !ok {
		t.Fatal("stale pinned version dropped")
	}
	part := &Plan{Partial: true}
	s.Put(k, part, now)
	if _, ok := s.Get(k, part.Version, now.Add(9*time.Minute)); !ok {
		t.Fatal("partial dropped early")
	}
	if _, ok := s.Get(k, part.Version, now.Add(11*time.Minute)); ok {
		t.Fatal("partial kept past 10 minutes")
	}
	if _, ok := s.Get(k, 3, time.Date(2026, 10, 24, 4, 0, 0, 0, time.UTC)); ok {
		t.Fatal("plan kept past 04:00 next day")
	}
}

func TestPlan_SeedOnceAndMixGate(t *testing.T) {
	rows := planRows(map[string]int{"a": 1, "b": 1, "c": 1}, nil)
	rows[0].Seed, rows[1].Seed = "x", "x"
	mix := Row{ID: "mix:1", Family: "mix", Prior: PriorPersonal, Min: 8}
	for k := 0; k < 7; k++ {
		mix.Items = append(mix.Items, Item{Key: fmt.Sprint("m", k), Artist: fmt.Sprint("ma", k), Score: 0.99})
	}
	got := NewPlan(append(rows, mix), nil, planMax, 1, "2026-10-23", nil).All(nil)
	seeds := 0
	for _, r := range got {
		if r.Seed == "x" {
			seeds++
		}
		if r.ID == "mix:1" {
			t.Fatal("a mix with 7 items passed its gate of 8")
		}
	}
	if seeds != 1 {
		t.Fatalf("seed x used by %d rows", seeds)
	}
}

func TestPlanStore_NewestServesStaleAndOld(t *testing.T) {
	var s PlanStore
	now := time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	y := PlanKey{Member: 3, Date: "2026-10-27", View: "US"}
	p := &Plan{}
	s.Put(y, p, now.Add(-10*time.Hour))
	if _, ok := s.Current(PlanKey{Member: 3, Date: "2026-10-28", View: "US"}, now); ok {
		t.Fatal("no plan for today expected")
	}
	if got, ok := s.Newest(3, "US", now); !ok || got != p {
		t.Fatal("yesterday's plan not served as the last good plan")
	}
	s.MarkStaleKey(y)
	if _, ok := s.Current(y, now); ok {
		t.Fatal("stale plan answered as current")
	}
	if got, ok := s.Newest(3, "US", now); !ok || got != p {
		t.Fatal("stale plan not kept for serving")
	}
	q := &Plan{Partial: true}
	s.Put(y, q, now.Add(-time.Hour))
	if got, ok := s.Newest(3, "US", now); !ok || got != q {
		t.Fatal("partial plan past PartialTTL not served while it is retried")
	}
	if _, ok := s.Newest(4, "US", now); ok {
		t.Fatal("another member's plan served")
	}
}

// planRows makes n rows per family with 8 items each. Families in disc are
// discovery rows. Scores fall with the row number so strength orders them.
func planRows(fams map[string]int, disc map[string]bool) []Row {
	var out []Row
	names := []string{}
	for f := range fams {
		names = append(names, f)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	for fi, f := range names {
		for n := 0; n < fams[f]; n++ {
			id := fmt.Sprintf("%s:%d", f, n)
			r := Row{ID: id, Family: f, Disc: disc[f], Prior: PriorMixed}
			for k := 0; k < 8; k++ {
				r.Items = append(r.Items, Item{Key: id + fmt.Sprint(k), Artist: fmt.Sprintf("%s-a%d", id, k), Score: 0.9 - 0.01*float64(fi+n)})
			}
			out = append(out, r)
		}
	}
	return out
}
