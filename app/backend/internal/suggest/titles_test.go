package suggest

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// genreItems makes n items whose genres cycle through the given sets.
func genreItems(n int, genres ...[]string) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{TMDBID: fmt.Sprint(i + 1), Kind: "movie", Name: fmt.Sprint("T", i), Genres: genres[i%len(genres)], Released: "2001-01-01"}
	}
	return out
}

// plainMoment is a Tuesday afternoon with no occasion.
var plainMoment = Moment{Local: time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC)}

// ctxAt builds a title context for a moment with nothing remembered.
func ctxAt(m Moment) TitleContext {
	return TitleContext{Member: 7, Moment: m, Day: m.Local.Format("2006-01-02"), Recent: map[string]string{}, Today: map[string]string{}}
}

// titleOne titles a single tail row.
func titleOne(r Row, c TitleContext) Row {
	pg, _ := TitlePage(Page{Tail: []Row{r}}, c)
	return pg.Tail[0]
}

// tmpl finds a template by id.
func tmpl(id string) Template {
	for _, t := range Templates {
		if t.ID == id {
			return t
		}
	}
	return Template{}
}

// TestTemplates_Library checks every template is well formed and unique.
func TestTemplates_Library(t *testing.T) {
	per := map[string]int{}
	newWord := regexp.MustCompile(`(?i)\b(new|fresh|recent(ly)?|just out|latest)\b`)
	for _, tp := range Templates {
		per[tp.Layer]++
		if strings.Contains(tp.Text, "!") {
			t.Errorf("%s has an exclamation mark", tp.Text)
		}
		for _, s := range tp.slots() {
			if !contains([]string{"genre", "genre2", "name_of_seed", "person", "day", "timeword", "decade", "service", "count", "title"}, s) {
				t.Errorf("%s: unknown slot %s", tp.Text, s)
			}
		}
		if tp.Kid && !KidSafe(tp.Text) {
			t.Errorf("Kid template with a scary word: %s", tp.Text)
		}
		if strings.Contains(strings.ToLower(tp.Text), "trend") && !(tp.Layer == LayerReason && tp.Key == TypeTrending) {
			t.Errorf("trending claimed outside a trending row: %s", tp.Text)
		}
		plain := strings.ReplaceAll(tp.Text, "New Year", "")
		plain = strings.ReplaceAll(plain, "New year", "")
		if newWord.MatchString(plain) && !tp.New && tp.Key != "release_day" && tp.Key != TypeNewServices {
			t.Errorf("claims new without the n flag: %s", tp.Text)
		}
		if len(strings.NewReplacer("{genre}", "", "{genre2}", "", "{day}", "", "{count}", "").Replace(tp.Text)) > TitleMax {
			t.Errorf("too long: %s", tp.Text)
		}
	}
	t.Logf("templates: %d %v", len(Templates), per)
	if len(Templates) < 450 {
		t.Fatalf("only %d templates", len(Templates))
	}
}

// TestMixOf_Thresholds checks the lead and pair limits.
func TestMixOf_Thresholds(t *testing.T) {
	h, c, d := []string{"Horror"}, []string{"Comedy"}, []string{"Drama"}
	if m := MixOf(genreItems(10, h, c)); m.Lead == "" {
		t.Fatalf("50%% horror should lead: %+v", m)
	}
	if m := MixOf(genreItems(10, h, c, d)); m.Lead != "" {
		t.Fatalf("a third should not lead: %+v", m)
	}
	items := genreItems(10, d)
	for i := 0; i < 4; i++ {
		items[i].Genres = h
	}
	for i := 4; i < 7; i++ {
		items[i].Genres = c
	}
	m := MixOf(items)
	if m.Lead != "" || m.Pair != [2]string{"horror", "comedy"} {
		t.Fatalf("pair: %+v", m)
	}
}

// TestTitlePage_TruthfulTitles checks a template never claims more than the row holds.
func TestTitlePage_TruthfulTitles(t *testing.T) {
	c := ctxAt(plainMoment)
	mixed := Row{ID: "hidden", Type: TypeHidden, Title: "Hidden gems", Items: genreItems(12, []string{"Horror"}, []string{"Comedy"}, []string{"Drama"})}
	for day := 0; day < 30; day++ {
		cc := c
		cc.Day = fmt.Sprint("d", day)
		got := titleOne(mixed, cc)
		if strings.Contains(strings.ToLower(got.Title), "horror") {
			t.Fatalf("horror claimed on a mixed row: %q", got.Title)
		}
	}
	horror := Row{ID: "genre:Horror", Type: TypeGenre, Genre: "Horror", Title: "Horror picks for you", Items: genreItems(12, []string{"Horror"})}
	seen := 0
	for _, tp := range Templates {
		f := factsOf(horror, c)
		text, ok := tp.fill(f, c)
		if !ok {
			continue
		}
		seen++
		lt := strings.ToLower(text)
		if tp.New || strings.Contains(lt, "trend") {
			t.Fatalf("untrue title on an old horror row: %q", text)
		}
		for _, g := range []string{"comedy", "romance", "family", "documentary"} {
			if strings.Contains(lt, g) {
				t.Fatalf("wrong genre on a horror row: %q", text)
			}
		}
	}
	if seen < 10 {
		t.Fatalf("only %d templates fit a horror row", seen)
	}
	fresh := horror
	fresh.Items = genreItems(12, []string{"Horror"})
	for i := range fresh.Items {
		fresh.Items[i].Released = plainMoment.Local.AddDate(0, 0, -10).Format("2006-01-02")
	}
	if text, ok := tmpl(findID(t, LayerGenre, "any", "New {genre}")).fill(factsOf(fresh, c), c); !ok || text != "New horror" {
		t.Fatalf("new horror: %q %v", text, ok)
	}
	if _, ok := tmpl(findID(t, LayerGenre, "any", "New {genre}")).fill(factsOf(horror, c), c); ok {
		t.Fatal("old row called new")
	}
	hw := ctxAt(Moment{Local: time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)})
	if _, ok := tmpl(findID(t, LayerOccasion, "halloween", "Halloween horror")).fill(factsOf(horror, hw), hw); !ok {
		t.Fatal("Halloween horror should fit in late October")
	}
	if _, ok := tmpl(findID(t, LayerOccasion, "halloween", "Halloween horror")).fill(factsOf(horror, c), c); ok {
		t.Fatal("Halloween horror in early October")
	}
	if _, ok := tmpl(findID(t, LayerOccasion, "halloween", "Halloween horror")).fill(factsOf(mixed, hw), hw); ok {
		t.Fatal("Halloween horror on a mixed row")
	}
	got := titleOne(horror, hw)
	if len(got.Items) != len(horror.Items) || got.Items[0].Key() != horror.Items[0].Key() {
		t.Fatal("titling changed the items")
	}
	because := Row{ID: "because_have:movie:1", Type: TypeBecauseHave, SeedName: "Alien", Title: "Because you have Alien", Items: genreItems(12, []string{"Science Fiction"})}
	for day := 0; day < 20; day++ {
		cc := c
		cc.Day = fmt.Sprint("d", day)
		if got := titleOne(because, cc); !strings.Contains(got.Title, "Alien") {
			t.Fatalf("because row lost its seed: %q", got.Title)
		}
	}
}

// findID finds the id of the template with this layer, key and text.
func findID(t *testing.T, layer, key, text string) string {
	t.Helper()
	for _, tp := range Templates {
		if tp.Layer == layer && tp.Key == key && tp.Text == text {
			return tp.ID
		}
	}
	t.Fatalf("no template %s/%s %q", layer, key, text)
	return ""
}

// TestTitlePage_LayerFallback checks the layers are tried from most specific down.
func TestTitlePage_LayerFallback(t *testing.T) {
	c := ctxAt(Moment{Local: time.Date(2026, 10, 23, 23, 0, 0, 0, time.UTC)})
	horror := Row{ID: "genre:Horror", Type: TypeGenre, Title: "Horror picks for you", Items: genreItems(12, []string{"Horror"})}
	if got := titleOne(horror, c); got.Layer != LayerOccasion {
		t.Fatalf("occasion first, got %s %q", got.Layer, got.Title)
	}
	for _, tp := range Templates {
		if tp.Layer == LayerOccasion {
			c.Recent[tp.ID] = c.Day
		}
	}
	if got := titleOne(horror, c); got.Layer != LayerTime {
		t.Fatalf("time next, got %s %q", got.Layer, got.Title)
	}
	for _, l := range []string{LayerTime, LayerGenre} {
		for _, tp := range Templates {
			if tp.Layer == l {
				c.Recent[tp.ID] = c.Day
			}
		}
	}
	got := titleOne(horror, c)
	if got.Layer != LayerReason {
		t.Fatalf("reason next (no taste), got %s %q", got.Layer, got.Title)
	}
	for _, tp := range Templates {
		c.Recent[tp.ID] = c.Day
	}
	if got := titleOne(horror, c); got.Layer != LayerPlain || got.Title != "Horror picks for you" {
		t.Fatalf("plain last, got %s %q", got.Layer, got.Title)
	}
	c2 := ctxAt(plainMoment)
	c2.Taste = []string{"Horror"}
	for _, l := range []string{LayerTime, LayerGenre} {
		for _, tp := range Templates {
			if tp.Layer == l {
				c2.Recent[tp.ID] = c2.Day
			}
		}
	}
	if got := titleOne(horror, c2); got.Layer != LayerTaste {
		t.Fatalf("taste, got %s %q", got.Layer, got.Title)
	}
	top := Row{ID: TypeCinemas, Type: TypeCinemas, Title: RowTitle(TypeCinemas, ""), Items: genreItems(10, []string{"Drama"})}
	c3 := ctxAt(plainMoment)
	pg, _ := TitlePage(Page{Head: []Row{top}}, c3)
	if pg.Head[0].Title != "Coming to cinemas and streaming" || pg.Head[0].Subtitle != "In the next 90 days" {
		t.Fatalf("fixed row: %+v", pg.Head[0])
	}
}

// TestTitlePage_KidTitles checks a Kid only gets safe templates.
func TestTitlePage_KidTitles(t *testing.T) {
	c := ctxAt(Moment{Local: time.Date(2026, 10, 23, 23, 30, 0, 0, time.UTC)})
	c.Kid = true
	rows := []Row{
		{ID: "a", Type: TypeHidden, Title: "Hidden gems", Items: genreItems(12, []string{"Horror"})},
		{ID: "b", Type: TypeBest, Title: "Best new releases", Items: genreItems(12, []string{"Animation", "Family"})},
		{ID: "c", Type: TypeGenre, Title: "Thriller picks for you", Items: genreItems(12, []string{"Thriller"})},
	}
	for day := 0; day < 30; day++ {
		c.Day = fmt.Sprint("d", day)
		pg, _ := TitlePage(Page{Tail: rows}, c)
		for _, r := range pg.Tail {
			if r.TitleID != "" && !tmpl(r.TitleID).Kid {
				t.Fatalf("adult template for a Kid: %q", r.Title)
			}
			if r.Layer != LayerPlain && !KidSafe(r.Title) {
				t.Fatalf("scary word for a Kid: %q", r.Title)
			}
		}
	}
}

// TestTitlePage_StableWithinDay checks titles hold for the whole local day.
func TestTitlePage_StableWithinDay(t *testing.T) {
	c := ctxAt(plainMoment)
	rows := simRows(0)
	a, shown := TitlePage(Page{Tail: rows}, c)
	b, _ := TitlePage(Page{Tail: rows}, c)
	c2 := ctxAt(plainMoment)
	for _, s := range shown {
		c2.Today[s.RowID] = s.TemplateID
		c2.Recent[s.TemplateID] = c.Day
	}
	d, _ := TitlePage(Page{Tail: rows}, c2)
	for i := range a.Tail {
		if a.Tail[i].Title != b.Tail[i].Title || a.Tail[i].Title != d.Tail[i].Title {
			t.Fatalf("row %d flickered: %q %q %q", i, a.Tail[i].Title, b.Tail[i].Title, d.Tail[i].Title)
		}
	}
	ids := map[string]bool{}
	for _, r := range a.Tail {
		if r.TitleID != "" {
			if ids[r.TitleID] {
				t.Fatalf("template twice on one page: %s", r.TitleID)
			}
			ids[r.TitleID] = true
		}
	}
}

// simGenres and simTypes feed the heavy user simulation.
var simGenres = [][]string{{"Horror"}, {"Comedy"}, {"Drama"}, {"Action"}, {"Thriller"}, {"Science Fiction"}, {"Fantasy"}, {"Romance"},
	{"Crime"}, {"Animation", "Family"}, {"Documentary"}, {"Mystery"}, {"Comedy", "Romance"}, {"Action", "Science Fiction"}, {"Drama", "Crime"}}

var simTypes = []string{TypeBecauseHave, TypeGenre, TypePerson, TypeDecade, TypeHidden, TypeBest, TypeNewServices, TypeHousehold, TypePopularGenre, TypePopularDecade}

// simRows is one heavy day of 50 rows of mixed types and genres.
func simRows(day int) []Row {
	var rows []Row
	for i := 0; i < 50; i++ {
		k := day*7 + i
		typ := simTypes[k%len(simTypes)]
		g := simGenres[(k*5+day)%len(simGenres)]
		r := Row{ID: fmt.Sprintf("%s:%d", typ, k%44), Type: typ, Items: genreItems(12, g)}
		switch typ {
		case TypeBecauseHave:
			r.SeedName = []string{"Alien", "Heat", "Up", "Fargo", "Arrival", "Coco", "Se7en", "Amélie"}[k%8]
		case TypePerson:
			r.Person = []string{"Greta Gerwig", "Denis Villeneuve", "Bong Joon-ho", "Pedro Pascal"}[k%4]
		case TypeDecade, TypePopularDecade:
			r.Decade = 1970 + 10*(k%5)
		}
		if typ == TypeBest || typ == TypeNewServices || k%6 == 0 {
			for j := range r.Items {
				r.Items[j].Released = "2026-10-01"
			}
		}
		r.Title = RowTitle(typ, r.SeedName+r.Person)
		rows = append(rows, r)
	}
	return rows
}

// TestTitlePage_HeavyUserNoRepeat checks a heavy user sees no template again within
// NoRepeatDays.
func TestTitlePage_HeavyUserNoRepeat(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	last := map[string]int{}
	distinct := map[string]bool{}
	smart, plain := 0, 0
	per := map[string]int{}
	for day := 0; day < 21; day++ {
		hour := []int{7, 10, 15, 20, 23}[day%5]
		m := Moment{Local: start.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour)}
		c := ctxAt(m)
		for id, d := range last {
			if day-d < NoRepeatDays {
				c.Recent[id] = fmt.Sprint(d)
			}
		}
		pg, shown := TitlePage(Page{Tail: simRows(day)}, c)
		for _, s := range shown {
			if d, ok := last[s.TemplateID]; ok && day-d < NoRepeatDays {
				t.Fatalf("day %d: %s again after %d days", day, s.TemplateID, day-d)
			}
			last[s.TemplateID] = day
			distinct[s.TemplateID] = true
		}
		for _, r := range pg.Tail {
			per[r.Layer]++
			if r.Layer == LayerPlain {
				plain++
			} else {
				smart++
			}
			if len([]rune(r.Title)) > TitleMax {
				t.Fatalf("too long: %q", r.Title)
			}
		}
	}
	t.Logf("21 days x 50 rows: %d smart titles, %d plain, %d distinct templates, by layer %v", smart, plain, len(distinct), per)
	if 10*smart < 4*21*50 {
		t.Fatalf("only %d of %d rows got a smart title", smart, 21*50)
	}
}

// TestMomentBoost_Bounded checks the boost is 1 or 1 + BoostMax.
func TestMomentBoost_Bounded(t *testing.T) {
	late := Moment{Local: time.Date(2026, 10, 6, 23, 30, 0, 0, time.UTC)}
	sat := Moment{Local: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	thr := Row{Items: genreItems(10, []string{"Thriller"})}
	fam := Row{Items: genreItems(10, []string{"Family"})}
	if MomentBoost(thr, late, false) != 1+BoostMax || MomentBoost(thr, late, true) != 1 {
		t.Fatal("late-night boost")
	}
	if MomentBoost(fam, sat, false) != 1+BoostMax || MomentBoost(fam, late, false) != 1 || MomentBoost(thr, Moment{}, false) != 1 {
		t.Fatal("weekend morning boost")
	}
	for _, m := range []Moment{late, sat, plainMoment} {
		for _, r := range []Row{thr, fam} {
			if b := MomentBoost(r, m, false); b < 1 || b > 1+BoostMax {
				t.Fatalf("boost out of bounds: %v", b)
			}
		}
	}
}

// TestTitlePage_SampleTitles titles a sample page.
func TestTitlePage_SampleTitles(t *testing.T) {
	moments := []Moment{
		{Local: time.Date(2026, 10, 23, 23, 0, 0, 0, time.UTC)},
		{Local: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)},
		{Local: time.Date(2026, 12, 12, 19, 0, 0, 0, time.UTC)},
		{Local: time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)},
	}
	for i, m := range moments {
		c := ctxAt(m)
		c.Taste = []string{"Drama"}
		pg, _ := TitlePage(Page{Tail: simRows(i)[:6]}, c)
		for _, r := range pg.Tail {
			t.Logf("%s %-8s %-40s | %s", m.Local.Format("Mon 15h"), r.Layer, r.Title, r.Subtitle)
		}
	}
}
