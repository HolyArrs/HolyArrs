package sources

import (
	"context"
	"errors"
	"hash/fnv"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// maxNameSearches is how many of a title's other names a hand search asks a
// Source about, and maxAutoNameSearches how many an automatic search does.
// Each is one more request to the Source, so the numbers stay small.
const (
	maxNameSearches     = 3
	maxAutoNameSearches = 1
)

// romanised are the languages not written in Latin letters. A title in one
// of them is released under several romanised spellings of its name.
var romanised = map[string]bool{
	"ja": true, "ko": true, "zh": true, "cn": true, "th": true, "hi": true, "ta": true, "te": true,
	"ml": true, "kn": true, "bn": true, "mr": true, "pa": true, "gu": true, "ur": true, "ar": true,
	"fa": true, "he": true, "ru": true, "uk": true, "be": true, "bg": true, "sr": true, "mk": true,
	"el": true, "ka": true, "hy": true, "km": true, "lo": true, "my": true, "ne": true, "si": true,
	"am": true, "mn": true, "kk": true,
}

// nameSearches is how many other names a search of the title asks a
// word-only Source about. An English title's other names are its names in
// other countries, where the releases are dubbed, so none is asked. Any other
// title's list starts with its own-language or romanised name. A language
// written in Latin letters has the one own name, and what follows it are
// names in other countries again, so a hand search asks more than one name
// only for a romanised title, or when the language is not known.
func nameSearches(ti mediafiles.Title, hand bool) int {
	language := ""
	if ti.OriginalLanguage != nil {
		language = *ti.OriginalLanguage
	}
	if language == "en" {
		return 0
	}
	if hand && (language == "" || romanised[language]) {
		return maxNameSearches
	}
	return maxAutoNameSearches
}

// nameAsk is which other names a search of a show asks a word-only Source
// about: how many, and shared, which says whether another Library show
// carries a name too, and whether one of the same year does. shared reads the
// Library on its first call, so it is called only when a Source is about to
// be asked by words. A movie asks its original and translated names, with
// its year, only when its own name found nothing on that Source, as the
// older movie app does.
type nameAsk struct {
	n      int
	wanted int // how many of the asked names are in the Version's wanted languages
	shared func(ctx context.Context, name string) (shared, sameYear bool)
}

// wantedNames are the title's kept names in the Version's wanted audio
// languages and the title language set in naming, other than English, as
// the older movie app adds the translations of a profile's wanted languages
// to every search: a French copy is released under the French name.
func wantedNames(ti mediafiles.Title, langs []string, titleLang string) []string {
	tmdb := ti.SourceIDs["tmdb"]
	if mediafiles.TitleNamer == nil || tmdb == "" || (ti.Kind != "movie" && ti.Kind != "show") {
		return nil
	}
	var codes []string
	for _, l := range append(append([]string{}, langs...), titleLang) {
		switch l = strings.ToLower(strings.TrimSpace(l)); l {
		case "", "en", "und":
		case "pt":
			codes = append(codes, "pt-br", "pt-pt")
		case "zh":
			codes = append(codes, "zh-cn", "zh-tw")
		case "no":
			codes = append(codes, "nb")
		default:
			codes = append(codes, l)
		}
	}
	var out []string
	seen := map[string]bool{strings.ToLower(ti.Name): true}
	for _, c := range codes {
		if n := strings.TrimSpace(mediafiles.TitleNamer(ti.Kind, tmdb, c)); n != "" && !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			out = append(out, n)
		}
	}
	return out
}

// titleLanguage is the title language set in naming ("" when unset or
// unreadable).
func (s *Searcher) titleLanguage(ctx context.Context) string {
	if s.Store == nil {
		return ""
	}
	ns, err := mediafiles.ReadNaming(ctx, s.Store.Read())
	if err != nil {
		return ""
	}
	return ns.TitleLanguage
}

// withWanted puts the wanted-language names first in q's other names and
// makes sure each is asked, up to maxNameSearches.
func withWanted(q *Query, wanted []string) {
	if len(wanted) == 0 {
		return
	}
	q.Aliases = append(append([]string{}, wanted...), q.Aliases...)
	q.other.wanted = min(len(wanted), maxNameSearches)
	q.other.n = max(q.other.n, q.other.wanted)
}

// nameAsk works out the other names to ask for a search of the title.
func (s *Searcher) nameAsk(ctx context.Context, ti mediafiles.Title, hand bool) nameAsk {
	if ti.Kind == "movie" {
		return nameAsk{n: nameSearches(ti, hand)}
	}
	if ti.Kind != "show" {
		return nameAsk{}
	}
	a := nameAsk{n: nameSearches(ti, hand)}
	var lib *sharedNames
	a.shared = func(ctx context.Context, name string) (bool, bool) {
		if lib == nil {
			lib = s.libraryNames(ctx)
		}
		return lib.has(name, ti.ID, ti.Year)
	}
	return a
}

// sharedNamesFor is how long the Library's show names are kept before they
// are let go, so a run of searches reads and cleans them once and they do not
// stay in memory after it. A show added or renamed in that time is seen by
// the next read.
const sharedNamesFor = 30 * time.Second

// libraryNames gives the Library's show names for looking one up. They are
// kept for sharedNamesFor and then let go. A read that fails gives no names
// and is not kept.
func (s *Searcher) libraryNames(ctx context.Context) *sharedNames {
	s.namesMu.Lock()
	defer s.namesMu.Unlock()
	keep := s.namesFor
	if keep <= 0 {
		keep = sharedNamesFor
	}
	if s.names != nil && time.Since(s.namesAt) <= keep {
		return s.names
	}
	shows, err := mediafiles.ShowNames(ctx, s.Store.Read())
	if err != nil {
		return newSharedNames(nil)
	}
	at := time.Now()
	s.names, s.namesAt = newSharedNames(shows), at
	time.AfterFunc(keep, func() {
		s.namesMu.Lock()
		defer s.namesMu.Unlock()
		if s.namesAt.Equal(at) {
			s.names = nil
		}
	})
	return s.names
}

// sharedNames is the names Library shows carry, each cleaned once into the
// forms the judge matches a release name against, for looking a name up. To
// stay small it keeps a number worked out from each form, not the form, and
// for it the shows that carry it: one, and the few more in more.
// All-capitals names, which also match their letters written apart, are kept
// under a number of their own.
type sharedNames struct {
	one     map[uint64]nameOwner
	more    map[uint64][]nameOwner
	cleaned int // Library names cleaned
}

// nameOwner is a show that carries a name: its id and year (0: not known).
type nameOwner struct {
	id   int32
	year int16
}

// nameKey is the number a cleaned name is kept under.
func nameKey(clean string, caps bool) uint64 {
	h := fnv.New64a()
	if caps {
		_, _ = h.Write([]byte{0})
	}
	_, _ = h.Write([]byte(clean))
	return h.Sum64()
}

func newSharedNames(shows []mediafiles.ShowName) *sharedNames {
	l := &sharedNames{one: make(map[uint64]nameOwner, len(shows)), more: map[uint64][]nameOwner{}}
	add := func(k uint64, o nameOwner) {
		if first, ok := l.one[k]; !ok {
			l.one[k] = o
		} else if first != o {
			l.more[k] = append(l.more[k], o)
		}
	}
	for _, sh := range shows {
		l.cleaned++
		o := nameOwner{id: int32(sh.ID), year: int16(sh.Year)}
		for _, f := range releasename.NameForms(sh.Name) {
			add(nameKey(f, false), o)
		}
		if strings.ToUpper(sh.Name) == sh.Name && strings.ToLower(sh.Name) != sh.Name {
			add(nameKey(releasename.CleanName(sh.Name), true), o)
		}
	}
	return l
}

// has says whether a Library show other than exceptID carries the name, and
// whether one of the given year (or of no known year) does.
func (l *sharedNames) has(name string, exceptID int64, year *int) (shared, sameYear bool) {
	clean := releasename.CleanName(name)
	if clean == "" {
		return false, false
	}
	for _, k := range []uint64{nameKey(clean, false), nameKey(releasename.JoinLetters(clean), true)} {
		first, ok := l.one[k]
		if !ok {
			continue
		}
		for _, o := range append([]nameOwner{first}, l.more[k]...) {
			if int64(o.id) != exceptID {
				shared = true
				sameYear = sameYear || o.year == 0 || year == nil || int(o.year) == *year
			}
		}
	}
	return shared, sameYear
}

// minNameLetters is the fewest letters an other name needs to be asked; a
// shorter one finds anything.
const minNameLetters = 3

// maxNameLen is the longest other name worth sending to a Source; a name
// this long is a sentence, not the show's name.
const maxNameLen = 120

// searchShare is how full a Source's search window may get before the other
// names are left out, so extra searches never use up the limit the title's own
// search needs.
const searchShare = 0.3

// searchNamed is Search plus what the Source holds under the title's other
// names, up to max of them. A Source asked by an id names the title however a
// release is named, also when it holds nothing, so only a word search asks
// the other names.
func (c *Client) searchNamed(ctx context.Context, q Query, max int) (items []Item, slowed, err error) {
	items, byID, err := c.search(ctx, q)
	if err != nil || byID {
		return items, nil, err
	}
	items, slowed = searchOtherNames(ctx, c, q, items, max)
	return items, slowed, nil
}

// searchOtherNames adds what a Source holds under the title's other names.
// Many sites search by words only, and a show is often released under its
// Japanese or original name, so the other names that read as plain Latin
// words are searched the same way (no ids), the first max of them, and what
// they find is added. A failing extra search keeps what was
// found; when the Source asked to slow down or blocked the search, that error
// is returned with the items so the caller can pause the Source.
func searchOtherNames(ctx context.Context, c *Client, q Query, items []Item, max int) ([]Item, error) {
	if max <= 0 || len(q.Aliases) == 0 || q.Music || q.feed || !q.TV && len(items) > 0 && q.other.wanted == 0 {
		return items, nil
	}
	// A name another Library show of the same year carries is not asked:
	// nothing could tell the two apart. With a show of another year, a year
	// in the release name does, so only releases that give one are kept.
	names := q.Aliases
	other := map[string]bool{} // asked names a show of another year carries
	if q.other.shared != nil {
		names = nil
		for _, n := range q.Aliases {
			shared, sameYear := q.other.shared(ctx, n)
			if shared && !sameYear {
				other[plainName(n)] = true
			}
			if !sameYear {
				names = append(names, n)
			}
		}
	}
	for _, text := range otherNameTexts(q.Title, names, max) {
		if ctx.Err() != nil || !c.hasRoom() {
			break
		}
		by := q
		by.Title, by.Aliases = text, nil
		by.IMDb, by.TMDb, by.TVDB, by.TVMaze = "", 0, 0, 0
		more, err := c.Search(ctx, by)
		if err != nil {
			var se *Error
			if errors.As(err, &se) && (se.Kind == KindRateLimited || se.Kind == KindBlocked) {
				return items, err
			}
			break
		}
		for _, it := range more {
			if !other[text] || releasename.Parse(it.Title, releasename.Hints{Size: it.Size, Show: true}).Year.Value > 0 {
				items = append(items, it)
			}
		}
	}
	return items, nil
}

// hasRoom says whether the Source's search window has space for one more
// search without passing searchShare of its limit.
func (c *Client) hasRoom() bool {
	if c.cfg.PerWindow <= 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	n := 0
	for _, h := range c.hits {
		if now.Sub(h) < c.cfg.Window {
			n++
		}
	}
	return float64(n+1) <= searchShare*float64(c.cfg.PerWindow)
}

// otherNameTexts picks the other names to search, in the order the metadata
// lists them (its other titles before its translations): plain Latin ones,
// leaving out a name that holds all the words of the title or of another name
// (a word search for the longer one only finds less), up to max.
func otherNameTexts(title string, aliases []string, max int) []string {
	own := strings.Fields(plainName(title))
	var names []string
	seen := map[string]bool{}
	for _, a := range aliases {
		if t := plainName(a); len(strings.ReplaceAll(t, " ", "")) >= minNameLetters && !seen[t] {
			seen[t] = true
			names = append(names, t)
		}
	}
	// A shorter name later in the list takes the place of a longer one that
	// holds all its words, so the place in the order is kept.
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if wi, wj := strings.Fields(names[i]), strings.Fields(names[j]); len(wj) < len(wi) && holdsAll(wi, wj) {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var out []string
	for i, t := range names {
		words := strings.Fields(t)
		redundant := len(own) > 0 && holdsAll(words, own)
		for j, u := range names {
			if uw := strings.Fields(u); j != i && holdsAll(words, uw) && (len(uw) < len(words) || j < i) {
				redundant = true
			}
		}
		if redundant {
			continue
		}
		if out = append(out, t); len(out) == max {
			break
		}
	}
	return out
}

// holdsAll says whether words has every word of want.
func holdsAll(words, want []string) bool {
	have := map[string]bool{}
	for _, w := range words {
		have[w] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}

// letterApostrophe finds a one-letter word joined to the next by an apostrophe
// ("L'Auberge", "d'Amélie").
var letterApostrophe = regexp.MustCompile("(^|[^\\pL\\pN])(\\pL)['’‘`]")

// plainName is a name as a word search sends it: lower case, accents and
// marks gone, a one-letter word before an apostrophe kept apart, and empty
// when it holds a letter outside plain Latin, which most sites cannot search,
// or is too long to be a name.
func plainName(name string) string {
	name = letterApostrophe.ReplaceAllString(name, "$1$2 ")
	clean := releasename.CleanName(strings.NewReplacer("'", "", "’", "", "‘", "", "`", "", "´", "", "＇", "").Replace(name))
	if len(clean) > maxNameLen {
		return ""
	}
	for _, r := range clean {
		if r > unicode.MaxASCII {
			return ""
		}
	}
	return clean
}
