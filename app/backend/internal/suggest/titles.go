package suggest

import (
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// titleFiles holds the template library, one JSON file per layer.
//
//go:embed titles/*.json
var titleFiles embed.FS

// Title layers, most specific first. LayerPlain marks a row that kept its plain
// title and LayerFixed a head row whose title never changes.
const (
	LayerOccasion = "occasion"
	LayerTime     = "time"
	LayerGenre    = "genre"
	LayerTaste    = "taste"
	LayerReason   = "reason"
	LayerPlain    = "plain"
	LayerFixed    = "fixed"
)

// TitleMax is the longest a filled title may be, in characters.
const TitleMax = 40

// NoRepeatDays is how long a template rests for a member once shown.
const NoRepeatDays = 14

// BoostMax is the most the moment raises a tail row's score. Row jitter is
// about 5%, so a boost moves a row a place or two and never to the top alone.
const BoostMax = 0.08

// Layers are the template layers in the order they are tried.
var Layers = []string{LayerOccasion, LayerTime, LayerGenre, LayerTaste, LayerReason}

// Templates is the whole library, loaded once at start.
var Templates = mustTemplates()

// layerCap is the most rows on one page a moment layer may title, so a Friday
// night does not turn every row into "Friday night ...".
var layerCap = map[string]int{LayerOccasion: 2, LayerTime: 3}

// slotRe finds the {slot} markers in a template text.
var slotRe = regexp.MustCompile(`\{([a-z_0-9]+)\}`)

// genreGroups maps a TMDB genre name to the title genre groups it counts for.
var genreGroups = map[string][]string{
	"Horror": {"horror"}, "Comedy": {"comedy"}, "Romance": {"romance"},
	"Action": {"action"}, "Adventure": {"action"}, "Action & Adventure": {"action"},
	"Thriller": {"thriller"}, "Science Fiction": {"scifi"}, "Sci-Fi & Fantasy": {"scifi", "fantasy"},
	"Fantasy": {"fantasy"}, "Drama": {"drama"}, "Crime": {"crime"}, "Mystery": {"mystery"},
	"Animation": {"animation"}, "Family": {"family"}, "Kids": {"family"},
	"Documentary": {"documentary"}, "Anime": {"anime"}, "War": {"war"}, "War & Politics": {"war"},
	"Western": {"western"}, "Music": {"music"},
}

// groupWord is how a genre group reads inside a title.
var groupWord = map[string]string{
	"horror": "horror", "comedy": "comedy", "romance": "romance", "action": "action",
	"thriller": "thriller", "scifi": "sci-fi", "fantasy": "fantasy", "drama": "drama",
	"crime": "crime", "mystery": "mystery", "animation": "animation", "family": "family",
	"documentary": "documentary", "anime": "anime", "war": "war", "western": "western", "music": "music",
}

// kidBanned matches words a Kid's titles never hold.
var kidBanned = regexp.MustCompile(`(?i)horror|scar[ey]|fright|after dark|lights off|creep|terror|blood|kill|murder|dare|bump|crime|thrill|tense|spooky`)

// fixedTypes are the row types that keep their title and get a subtitle.
var fixedTypes = map[string]bool{TypeTopServices: true, TypeTopPicks: true, TypeTrending: true, TypeBest: true, TypeBecauseHave: true, TypeBecauseAsked: true,
	TypeCollection: true, TypeCinemas: true, TypeNewSeasons: true, TypeReturning: true, TypeClassics: true}

// requiredSlot is the slot the titles of a row type must name.
var requiredSlot = map[string]string{TypeBecauseHave: "name_of_seed", TypePerson: "person", TypeDecade: "decade", TypePopularDecade: "decade"}

// slotOwner is the row types a slot belongs to. Other rows cannot fill it.
var slotOwner = map[string][]string{"name_of_seed": {TypeBecauseHave}, "person": {TypePerson}, "decade": {TypeDecade, TypePopularDecade}}

// TitleContext is what titling one page knows.
type TitleContext struct {
	Member int64
	// Kid says the template may show on a Kid account.
	Kid    bool
	Moment Moment
	// Day is the viewer's local date (YYYY-MM-DD).
	Day string
	// Taste is the member's strongest genres (TMDB names).
	Taste []string
	// Recent maps template id to the newest day shown, within NoRepeatDays.
	Recent map[string]string
	// Today maps row id to the template id already chosen today.
	Today map[string]string
	// Service names the member's one service, when there is just one.
	Service string
}

// Template is one title pattern.
type Template struct {
	ID    string
	Layer string
	Key   string
	Text  string
	Kid   bool
	// New says the template claims the row is new.
	New bool
	// Groups are genre-group sets the row must lead with (each set: any of).
	Groups [][]string
	// Types limits the template to these row types.
	Types []string
}

// Shown is one template a page showed, kept so it can rest.
type Shown struct {
	RowID      string
	TemplateID string
}

// GenreMix is what a row's items say about genres. Lead is the group held by
// at least half the items. Pair is two groups that together cover 70% of the
// items, each at least 30%. Either may be empty.
type GenreMix struct {
	Lead string
	Pair [2]string
}

// Moment is the viewer's local time.
type Moment struct {
	Local time.Time
}

// rowFacts are a row's fillable slots and what is true of it.
type rowFacts struct {
	r       Row
	mix     GenreMix
	slots   map[string]string
	newRow  bool
	release bool
	taste   bool
}

// TitlePage gives the page's rows their titles. Fixed head rows keep their
// title and get a subtitle. The rest get the most specific template that fits,
// has rested NoRepeatDays and is not on the page already, else their plain
// title. It returns a copy and the templates it showed.
func TitlePage(pg Page, c TitleContext) (Page, []Shown) {
	out := pg
	out.Head = append([]Row(nil), pg.Head...)
	out.Tail = append([]Row(nil), pg.Tail...)
	used := map[string]bool{}
	perLayer := map[string]int{}
	var shown []Shown
	title := func(r *Row) {
		if fixedTypes[r.Type] {
			r.Layer = LayerFixed
			r.Subtitle = fixedSubtitle(*r, c)
			return
		}
		if r.Plain == "" {
			r.Plain = r.Title
		}
		f := factsOf(*r, c)
		pick := func(t Template, text string) {
			r.Title, r.TitleID, r.Layer = text, t.ID, t.Layer
			r.Counted = 0
			if t.has("count") {
				r.Counted = len(r.Items)
			}
			// A moment title replaces the plain one, so the plain one moves to the subtitle.
			if t.Layer == LayerOccasion || t.Layer == LayerTime {
				r.Subtitle = r.Plain
			}
			used[t.ID] = true
			perLayer[t.Layer]++
			shown = append(shown, Shown{RowID: r.ID, TemplateID: t.ID})
		}
		// Keep the title chosen earlier today, so a title holds for the whole day.
		if id := c.Today[r.ID]; id != "" && !used[id] {
			for _, t := range Templates {
				if t.ID == id {
					if text, ok := t.fill(f, c); ok {
						pick(t, text)
						return
					}
				}
			}
		}
		for _, layer := range Layers {
			if cp, ok := layerCap[layer]; ok && perLayer[layer] >= cp {
				continue
			}
			var best *Template
			var bestText string
			var bestH uint64
			for i := range Templates {
				t := &Templates[i]
				if t.Layer != layer || used[t.ID] || c.Recent[t.ID] != "" {
					continue
				}
				text, ok := t.fill(f, c)
				if !ok {
					continue
				}
				if h := titleHash(c.Member, c.Day, r.ID, t.ID); best == nil || h < bestH {
					best, bestText, bestH = t, text, h
				}
			}
			if best != nil {
				pick(*best, bestText)
				return
			}
		}
		r.Title, r.TitleID, r.Layer, r.Counted = r.Plain, "", LayerPlain, 0
	}
	for i := range out.Head {
		title(&out.Head[i])
	}
	for i := range out.Tail {
		title(&out.Tail[i])
	}
	return out, shown
}

// MomentBoost returns 1 + BoostMax when the row's genre mix fits the moment,
// else 1. Late at night it is thriller, horror or sci-fi (never for a Kid). On
// a weekend morning it is comedy, family or animation.
func MomentBoost(r Row, m Moment, kid bool) float64 {
	if m.Local.IsZero() {
		return 1
	}
	leads := MixOf(r.Items).Leads()
	any := func(gs ...string) bool {
		for _, g := range gs {
			if contains(leads, g) {
				return true
			}
		}
		return false
	}
	part := m.Part()
	switch {
	case part == "late_night" && !kid && any("thriller", "horror", "scifi"):
		return 1 + BoostMax
	case m.Weekend() && (part == "morning" || part == "early_morning") && any("comedy", "family", "animation"):
		return 1 + BoostMax
	}
	return 1
}

// MixOf measures the genre mix of items.
func MixOf(items []Item) GenreMix {
	n := len(items)
	if n == 0 {
		return GenreMix{}
	}
	sets := make([][]string, n)
	count := map[string]int{}
	for i, it := range items {
		sets[i] = GenreGroups(it.Genres)
		for _, g := range sets[i] {
			count[g]++
		}
	}
	var groups []string
	for g := range count {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if count[groups[i]] != count[groups[j]] {
			return count[groups[i]] > count[groups[j]]
		}
		return groups[i] < groups[j]
	})
	var m GenreMix
	if len(groups) > 0 && 2*count[groups[0]] >= n {
		m.Lead = groups[0]
	}
	best := 0
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			a, b := groups[i], groups[j]
			if 10*count[a] < 3*n || 10*count[b] < 3*n {
				continue
			}
			u := 0
			for _, s := range sets {
				if contains(s, a) || contains(s, b) {
					u++
				}
			}
			if 10*u >= 7*n && u > best {
				best, m.Pair = u, [2]string{a, b}
			}
		}
	}
	return m
}

// GenreGroups returns the deduplicated title groups of TMDB genre names.
func GenreGroups(genres []string) []string {
	var out []string
	for _, g := range genres {
		for _, gr := range genreGroups[g] {
			if !contains(out, gr) {
				out = append(out, gr)
			}
		}
	}
	return out
}

// KidSafe reports whether text may show on a Kid account.
func KidSafe(text string) bool { return !kidBanned.MatchString(text) }

// Leads returns every group the row may be called by.
func (m GenreMix) Leads() []string {
	var out []string
	if m.Lead != "" {
		out = append(out, m.Lead)
	}
	for _, g := range m.Pair {
		if g != "" && !contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

// Part returns the part of day: early_morning 5-8, morning 8-12, afternoon
// 12-17, evening 17-22, late_night 22-5.
func (m Moment) Part() string {
	h := m.Local.Hour()
	switch {
	case h >= 5 && h < 8:
		return "early_morning"
	case h >= 8 && h < 12:
		return "morning"
	case h >= 12 && h < 17:
		return "afternoon"
	case h >= 17 && h < 22:
		return "evening"
	}
	return "late_night"
}

// Weekend reports whether it is Saturday or Sunday.
func (m Moment) Weekend() bool {
	d := m.Local.Weekday()
	return d == time.Saturday || d == time.Sunday
}

// Occasions returns the dated moments of now, most specific first. release_day
// is not here because it is decided per row.
func (m Moment) Occasions() []string {
	t := m.Local
	mo, d := t.Month(), t.Day()
	var out []string
	switch {
	case mo == time.October && d >= 15:
		out = append(out, "halloween")
	case mo == time.February && d >= 10 && d <= 14:
		out = append(out, "valentines")
	case (mo == time.December && d >= 30) || (mo == time.January && d <= 3):
		out = append(out, "newyear")
	case mo == time.December && d <= 26:
		out = append(out, "holiday")
	case (mo == time.June && d >= 21) || mo == time.July || mo == time.August:
		out = append(out, "summer")
	}
	if t.Weekday() == time.Friday && t.Hour() >= 17 {
		out = append(out, "friday_night")
	}
	if m.Weekend() {
		out = append(out, "weekend")
	}
	return out
}

// fixedSubtitle returns a head row's small line. It is always a plain fact.
func fixedSubtitle(r Row, c TitleContext) string {
	switch r.Type {
	case TypeTopPicks:
		return "Matched to your Library and asks"
	case TypeTrending:
		if g := MixOf(r.Items).Lead; g != "" && !(c.Kid && g == "horror") {
			return "This week, mostly " + groupWord[g]
		}
		return "What people watch this week"
	case TypeCollection:
		return "The parts missing from collections you have"
	case TypeCinemas:
		return "In the next 90 days"
	case TypeNewSeasons:
		return "Shows you have that are airing, then new shows"
	case TypeReturning:
		return "Back in the next two months"
	}
	return ""
}

// factsOf collects the slots a row can fill and the truths a template may claim.
func factsOf(r Row, c TitleContext) rowFacts {
	f := rowFacts{r: r, mix: MixOf(r.Items), slots: map[string]string{}}
	if g := f.mix.Lead; g != "" {
		f.slots["genre"] = groupWord[g]
		if f.mix.Pair[0] != "" && (f.mix.Pair[0] == g || f.mix.Pair[1] == g) {
			other := f.mix.Pair[0]
			if other == g {
				other = f.mix.Pair[1]
			}
			f.slots["genre2"] = groupWord[other]
		}
	} else if f.mix.Pair[0] != "" {
		f.slots["genre"], f.slots["genre2"] = groupWord[f.mix.Pair[0]], groupWord[f.mix.Pair[1]]
	}
	// A Kid is never told a row is horror.
	if c.Kid && (f.mix.Lead == "horror" || f.mix.Pair[0] == "horror" || f.mix.Pair[1] == "horror") {
		delete(f.slots, "genre")
		delete(f.slots, "genre2")
	}
	switch r.Type {
	case TypeBecauseHave:
		f.slots["name_of_seed"] = r.SeedName
	case TypePerson:
		f.slots["person"] = r.Person
	case TypeDecade, TypePopularDecade:
		if r.Decade > 0 {
			f.slots["decade"] = strconv.Itoa(r.Decade) + "s"
		}
	}
	f.slots["day"] = c.Moment.Local.Weekday().String()
	if c.Service != "" && r.Type == TypeNewServices {
		f.slots["service"] = c.Service
	}
	// A count in a title only reads true for a fuller row.
	if n := len(r.Items); n >= 8 {
		f.slots["count"] = strconv.Itoa(n)
	}
	day := c.Moment.Local.Format("2006-01-02")
	recent := 0
	for _, it := range r.Items {
		t, err := time.Parse("2006-01-02", it.Released)
		if err != nil {
			continue
		}
		// New means released within 90 days, with a day of slack for time zones.
		if age := c.Moment.Local.Sub(t).Hours() / 24; age >= -1 && age <= 90 {
			recent++
		}
	}
	f.newRow = len(r.Items) > 0 && 2*recent >= len(r.Items)
	if len(r.Items) > 0 && r.Items[0].Released == day && r.Items[0].Name != "" {
		f.release = true
		f.slots["title"] = r.Items[0].Name
	}
	tasteGroups := GenreGroups(c.Taste)
	if g := f.mix.Lead; g != "" && contains(tasteGroups, g) {
		f.taste = true
	}
	return f
}

// fill returns the filled title, or false when the template does not fit.
func (t Template) fill(f rowFacts, c TitleContext) (string, bool) {
	if c.Kid && (!t.Kid || !KidSafe(t.Text)) {
		return "", false
	}
	if len(t.Types) > 0 && !contains(t.Types, f.r.Type) {
		return "", false
	}
	if t.New && !f.newRow {
		return "", false
	}
	if req := requiredSlot[f.r.Type]; req != "" && !t.has(req) {
		return "", false
	}
	for slot, owners := range slotOwner {
		if t.has(slot) && !contains(owners, f.r.Type) {
			return "", false
		}
	}
	leads := f.mix.Leads()
	if c.Kid {
		leads = without(leads, "horror")
	}
	for _, set := range t.Groups {
		ok := false
		for _, g := range set {
			ok = ok || contains(leads, g)
		}
		if !ok {
			return "", false
		}
	}
	switch t.Layer {
	case LayerReason:
		if t.Key != f.r.Type {
			return "", false
		}
	case LayerGenre:
		switch t.Key {
		case "any":
			if f.mix.Lead == "" {
				return "", false
			}
		case "pair":
			if f.mix.Pair[0] == "" {
				return "", false
			}
		default:
			if !contains(leads, t.Key) {
				return "", false
			}
		}
	case LayerTaste:
		if !f.taste || (t.Key != "any" && t.Key != f.mix.Lead) {
			return "", false
		}
	case LayerTime:
		if t.Key != c.Moment.Part() {
			return "", false
		}
	case LayerOccasion:
		if t.Key == "release_day" {
			if !f.release {
				return "", false
			}
		} else if !contains(c.Moment.Occasions(), t.Key) {
			return "", false
		}
	}
	// A title with an empty slot would read wrong, so it does not fit.
	out := t.Text

	for _, s := range t.slots() {
		v := f.slots[s]
		if v == "" {
			return "", false
		}
		out = strings.ReplaceAll(out, "{"+s+"}", v)
	}
	out = capFirst(out)
	if utf8.RuneCountInString(out) > TitleMax {
		return "", false
	}
	if c.Kid && !KidSafe(out) {
		return "", false
	}
	return out, true
}

// slots lists the slot names used in the text.
func (t Template) slots() []string {
	var out []string
	for _, m := range slotRe.FindAllStringSubmatch(t.Text, -1) {
		out = append(out, m[1])
	}
	return out
}

// has reports whether the text uses the slot.
func (t Template) has(slot string) bool { return strings.Contains(t.Text, "{"+slot+"}") }

// mustTemplates loads the library and panics on a bad file, so a broken
// library is caught at start.
func mustTemplates() []Template {
	out, err := loadTemplates()
	if err != nil {
		panic(err)
	}
	return out
}

// loadTemplates reads titles/*.json. Each line is key|text|flags, with flags k
// (Kid ok), n (claims new), g=a/b (lead group) and t=a/b (row types).
func loadTemplates() ([]Template, error) {
	entries, err := titleFiles.ReadDir("titles")
	if err != nil {
		return nil, err
	}
	var out []Template
	seen := map[string]bool{}
	for _, e := range entries {
		b, err := titleFiles.ReadFile("titles/" + e.Name())
		if err != nil {
			return nil, err
		}
		var f struct {
			Layer     string   `json:"layer"`
			Templates []string `json:"templates"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		for _, line := range f.Templates {
			parts := strings.Split(line, "|")
			if len(parts) != 3 {
				return nil, fmt.Errorf("%s: bad template %q", e.Name(), line)
			}
			t := Template{Layer: f.Layer, Key: parts[0], Text: parts[1]}
			for _, fl := range strings.Split(parts[2], ",") {
				switch {
				case fl == "":
				case fl == "k":
					t.Kid = true
				case fl == "n":
					t.New = true
				case strings.HasPrefix(fl, "g="):
					t.Groups = append(t.Groups, strings.Split(fl[2:], "/"))
				case strings.HasPrefix(fl, "t="):
					t.Types = strings.Split(fl[2:], "/")
				default:
					return nil, fmt.Errorf("%s: bad flag %q in %q", e.Name(), fl, line)
				}
			}
			h := fnv.New32a()
			h.Write([]byte(t.Text))
			t.ID = fmt.Sprintf("%s.%s.%08x", t.Layer, t.Key, h.Sum32())
			if seen[t.ID] {
				return nil, fmt.Errorf("%s: template %q twice", e.Name(), t.Text)
			}
			seen[t.ID] = true
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return layerRank(out[i].Layer) < layerRank(out[j].Layer) })
	return out, nil
}

// layerRank orders layers by when they are tried. Unknown layers sort last.
func layerRank(l string) int {
	for i, x := range Layers {
		if x == l {
			return i
		}
	}
	return len(Layers)
}

// capFirst upper-cases the first letter.
func capFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

// without returns list minus s.
func without(list []string, s string) []string {
	out := []string{}
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// titleHash gives a stable order among templates for one member, day and row,
// so the pick holds all day.
func titleHash(member int64, day, row, id string) uint64 {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(member))
	h.Write(b[:])
	h.Write([]byte("|" + day + "|" + row + "|" + id))
	return h.Sum64()
}
