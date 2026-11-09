package suggest

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestHeadOrder_Tabs(t *testing.T) {
	for _, c := range []struct {
		sw   string
		want []string
	}{
		{"all", []string{TypeTopServices, TypeTopPicks, TypeTrending, TypeBecauseHave, TypeBest, TypeBecauseAsked, TypeHousehold}},
		{"movies", []string{TypeTopServices, TypeTopPicks, TypeCollection, TypeTrending, TypeBecauseHave, TypeBest, TypeCinemas, TypeHousehold}},
		{"shows", []string{TypeTopServices, TypeTopPicks, TypeNewSeasons, TypeTrending, TypeBecauseHave, TypeReturning, TypeHousehold}},
	} {
		t.Run(c.sw, func(t *testing.T) {
			p, _ := build(t, fullInput(c.sw, 4))
			svc := svcTopOf(p)
			pg := AssembleTabAt(p, svc, 4, Moment{})
			got := headIDs(pg.Head)
			if len(got) < len(c.want) || !slices.Equal(got[:len(c.want)], c.want) {
				t.Fatalf("head %v, want %v first", got, c.want)
			}
			if len(pg.Head) > HeadRows {
				t.Fatalf("%d head rows", len(pg.Head))
			}
			checkPage(t, c.sw, pg, svc)
			for _, r := range pg.Head {
				if r.Type == TypeTrending && c.sw != "all" && r.Title != "Trending "+c.sw+" this week" {
					t.Fatalf("trending title %q", r.Title)
				}
				if r.Type == TypeTopServices && c.sw == "all" && r.Title != "Trending on streaming" {
					t.Fatalf("services title %q", r.Title)
				}
			}
		})
	}
}

// A collection is one franchise, but the row keeps every missing part.
func TestAssemble_FinishTheCollection(t *testing.T) {
	in := fullInput("movies", 4)
	parts, _ := (&fakeSource{}).List(context.Background(), Request{List: "collection", Kind: "movie", Ref: "77", Scope: ScopeShared})
	in.Owned[parts[0].Key()] = true
	p, _ := build(t, in)
	pg := AssembleTabAt(p, nil, 4, Moment{})
	for _, r := range pg.Head {
		if r.Type != TypeCollection {
			continue
		}
		if len(r.Items) < 10 {
			t.Fatalf("collection row has %d items", len(r.Items))
		}
		for _, it := range r.Items {
			if it.Key() == parts[0].Key() || it.Collection != 77 {
				t.Fatalf("collection item %+v", it)
			}
		}
		return
	}
	t.Fatalf("no collection row in %v", headIDs(pg.Head))
}

func TestHeadOrder_Cold(t *testing.T) {
	for _, sw := range []string{"all", "movies", "shows"} {
		t.Run(sw, func(t *testing.T) {
			p, _ := build(t, Input{Member: 7, Switch: sw})
			svc := svcTopOf(p)
			pg := AssembleTabAt(p, svc, 7, Moment{})
			got := slotsOf(pg.Head)
			if got[0] != TypeTopServices {
				t.Fatalf("head %v", got)
			}
			for _, want := range []string{TypeTrending, TypeBest, TypePopularGenre, TypeClassics} {
				if !slices.Contains(got, want) {
					t.Fatalf("cold head %v has no %s", got, want)
				}
			}
			if sw == "all" && !slices.Equal(got[:6], []string{TypeTopServices, TypePopularGenre, TypeTrending, TypePopularGenre, TypeBest, TypeClassics}) {
				t.Fatalf("cold all head %v", got)
			}
			for _, r := range append(append([]Row{}, pg.Head...), pg.Tail...) {
				if Personal(r.Type) || r.Type == TypeHousehold {
					t.Fatalf("cold page has %s", r.ID)
				}
			}
			checkPage(t, "cold "+sw, pg, svc)
		})
	}
}

func TestHeadOrder_Kid(t *testing.T) {
	for _, c := range []struct {
		name  string
		allow func(Item) bool
		hh    bool
	}{
		{"all allowed", func(Item) bool { return true }, true},
		{"one household title refused", func(it Item) bool { return it.TMDBID != "8003" }, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := fullInput("all", 9)
			in.Kid, in.Allow = true, c.allow
			p, _ := build(t, in)
			pg := AssembleTabAt(p, nil, 9, Moment{})
			got := headIDs(pg.Head)
			want := []string{TypeTopServices, TypeTopPicks, TypePopularGenre + ":Family", TypePopularGenre + ":Animation", TypeBecauseHave}
			if c.hh {
				want = append(want, TypeHousehold)
			}
			if len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
				t.Fatalf("kid head %v, want %v first", got, want)
			}
			if !c.hh && slices.Contains(got, TypeHousehold) {
				t.Fatalf("household shown with a refused title: %v", got)
			}
			if len(pg.Head) > KidHeadRows || len(pg.Tail) > 8 {
				t.Fatalf("kid page %d head, %d tail rows", len(pg.Head), len(pg.Tail))
			}
			for _, r := range pg.Head {
				if r.Type == TypeTrending || r.Type == TypeBest {
					t.Fatalf("kid head has chart %s", r.Type)
				}
			}
			checkPage(t, "kid", pg, nil)
		})
	}
}

func TestAssemble_DedupeSpreadForGeneratedMembers(t *testing.T) {
	for _, sw := range []string{"all", "movies", "shows"} {
		for m := int64(1); m <= 40; m++ {
			in := fullInput(sw, m)
			in.Seeds = seedsN(int(m%12), m)
			if m%5 == 0 {
				in.Household, in.MyAsks, in.Collections = nil, nil, nil
			}
			p, _ := build(t, in)
			svc := svcTopOf(p)
			checkPage(t, fmt.Sprintf("%s member %d", sw, m), AssembleTabAt(p, svc, m, Moment{}), svc)
		}
	}
}

func TestSpread_MovesChartDown(t *testing.T) {
	rows := []Row{{ID: "s", Type: TypeTopServices}, {ID: "t", Type: TypeTrending}, {ID: "b", Type: TypeBest}, {ID: "g1", Type: TypePopularGenre}, {ID: "g2", Type: TypePopularGenre}, {ID: "c", Type: TypeClassics}}
	if got := order(spread(rows)); !slices.Equal(got, []string{"s", "g1", "t", "g2", "b", "c"}) {
		t.Fatalf("spread %v", got)
	}
	calm := []Row{{ID: "s", Type: TypeTopServices}, {ID: "p", Type: TypeTopPicks}, {ID: "t", Type: TypeTrending}}
	if got := order(spread(calm)); !slices.Equal(got, []string{"s", "p", "t"}) {
		t.Fatalf("spread changed a calm head: %v", got)
	}
}

func TestAssemble_HouseholdRowNoNames(t *testing.T) {
	in := fullInput("all", 4)
	p, _ := build(t, in)
	for _, r := range p.Rows {
		if r.Type != TypeHousehold {
			continue
		}
		if r.Person != "" || r.SeedName != "" {
			t.Fatalf("household row names someone: %+v", r)
		}
		for _, it := range r.Items {
			if len(it.People) > 0 {
				t.Fatalf("household item carries people: %+v", it)
			}
		}
		return
	}
	t.Fatal("no household row")
}

// A chart nearly all on Trending on streaming still shows, rather than vanish.
func TestAssemble_ChartMostlyOnServices(t *testing.T) {
	p, _ := build(t, fullInput("all", 4))
	var trending []Item
	for _, r := range p.Rows {
		if r.Type == TypeTrending {
			trending = r.Items
		}
	}
	svc := trending[:len(trending)-3]
	pg := AssembleTabAt(p, svc, 4, Moment{})
	if !slices.Contains(slotsOf(pg.Head), TypeTrending) {
		t.Fatalf("trending gone: %v", slotsOf(pg.Head))
	}
}

func TestAssemble_OldPoolRowsDropped(t *testing.T) {
	p, _ := build(t, fullInput("all", 4))
	p.Rows = append(p.Rows, Row{ID: "just_arrived", Type: "just_arrived", Items: genreItems(10, []string{"Drama"})})
	pg := AssembleTabAt(p, nil, 4, Moment{})
	for _, r := range append(append([]Row{}, pg.Head...), pg.Tail...) {
		if r.Type == "just_arrived" {
			t.Fatal("old row shown")
		}
	}
}

func TestAssemble_NoSameTypeBackToBackInHead(t *testing.T) {
	for _, sw := range []string{"all", "movies", "shows"} {
		for m := int64(1); m <= 20; m++ {
			in := fullInput(sw, m)
			in.Seeds = seedsN(10, m)
			p, _ := build(t, in)
			pg := AssembleTabAt(p, svcTopOf(p), m, Moment{})
			all := append(append([]Row{}, pg.Head...), pg.Tail...)
			for i := 1; i < len(all); i++ {
				if all[i].Type != all[i-1].Type {
					continue
				}
				other := false
				for _, r := range all[i:] {
					other = other || r.Type != all[i].Type
				}
				if other {
					t.Fatalf("%s member %d: rows %d and %d are both %s", sw, m, i-1, i, all[i].Type)
				}
			}
		}
	}
}

func TestAssemble_HeadRowsKeepTitles(t *testing.T) {
	pg := Page{Head: []Row{{ID: "best", Type: TypeBest, Title: "Best new releases"}, {ID: "because_have:movie:1", Type: TypeBecauseHave, SeedName: "Adolescence", Title: "Because you have Adolescence"}}}
	got, _ := TitlePage(pg, TitleContext{Day: "2026-10-20"})
	if got.Head[0].Title != "Best new releases" || got.Head[1].Title != "Because you have Adolescence" {
		t.Fatalf("titles %q, %q", got.Head[0].Title, got.Head[1].Title)
	}
}

func fullInput(sw string, member int64) Input {
	in := Input{Member: member, Switch: sw, Seeds: seedsN(6, member), Services: "8|9", Region: "US", Members: 3,
		Household: append(household(6), showHousehold(6)...), Collections: []LibCollection{{TMDBID: "77", Name: "Saga", Owned: 2}},
		MyAsks: []Item{{TMDBID: "9100", Kind: "movie", Name: "Asked movie"}, {TMDBID: "9101", Kind: "show", Name: "Asked show"}},
		Owned:  map[string]bool{}}
	for i := 0; i < 6; i++ {
		in.Returning = append(in.Returning, Item{TMDBID: fmt.Sprint(7000 + i), Kind: "show", Name: fmt.Sprint("Back", i), Owned: true, Released: "2026-11-20"})
	}
	airing, _ := (&fakeSource{}).List(context.Background(), Request{List: "upcoming", Kind: "show", Scope: ScopeShared})
	for _, it := range airing[:10] {
		in.Owned[it.Key()] = true
	}
	return in
}

func showHousehold(n int) []HouseholdTitle {
	out := household(n)
	for i := range out {
		out[i].Item.Kind = "show"
	}
	return out
}

// svcTopOf shares titles with the trending chart so the dedupe is exercised.
func svcTopOf(p Pool) []Item {
	for _, r := range p.Rows {
		if r.Type == TypeTrending {
			return append([]Item{}, r.Items[:min(6, len(r.Items))]...)
		}
	}
	return nil
}

func headIDs(rows []Row) []string {
	var out []string
	for _, r := range rows {
		if r.Type == TypePopularGenre {
			out = append(out, r.ID)
			continue
		}
		out = append(out, r.Type)
	}
	return out
}

// checkPage holds the rules every page keeps: one Trending on streaming row,
// no old Home rows, no repeat in the first HeadDedupe rows, and no two charts
// side by side in the first SpreadWindow rows.
func checkPage(t *testing.T, name string, pg Page, svcTop []Item) {
	t.Helper()
	rows := append(append([]Row{}, pg.Head...), pg.Tail...)
	n := 0
	for _, r := range rows {
		if r.Type == TypeTopServices {
			n++
		}
		if MinItems(r.Type) == 0 {
			t.Fatalf("%s: unknown row %s", name, r.Type)
		}
	}
	if n != 1 {
		t.Fatalf("%s: %d Trending on streaming rows", name, n)
	}
	seen := map[string]string{}
	for _, it := range svcTop {
		seen[it.Key()] = TypeTopServices
	}
	at := 0
	for _, r := range rows {
		if r.Type == TypeTopServices {
			continue
		}
		if at == HeadDedupe {
			break
		}
		at++
		for _, it := range r.Items {
			if prev, ok := seen[it.Key()]; ok {
				t.Fatalf("%s: %s in %s and %s", name, it.Key(), prev, r.ID)
			}
			seen[it.Key()] = r.ID
		}
	}
	for i := 1; i < min(SpreadWindow, len(pg.Head)); i++ {
		if Popular(pg.Head[i].Type) && Popular(pg.Head[i-1].Type) {
			t.Fatalf("%s: charts %s and %s side by side in %v", name, pg.Head[i-1].Type, pg.Head[i].Type, headIDs(pg.Head))
		}
	}
}
