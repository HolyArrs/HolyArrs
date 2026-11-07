// Package plainwords reads a search typed in plain words, like "that space
// movie with the robot from 2014", into a name and filters. It never fails:
// words it does not know stay in the name or the extra words.
package plainwords

import (
	"strconv"
	"strings"
	"unicode"
)

// Query is what a plain-words search was understood as.
type Query struct {
	// Text is the search as typed (trimmed, cut to a sane length).
	Text string `json:"text"`
	// Plain is true when nothing was understood: the whole text is a name.
	Plain bool `json:"plain"`
	// Name is what is left to match titles by name ("" when only filters).
	Name string `json:"name"`
	// Kind is movie, show or music, "" for any.
	Kind     string   `json:"kind,omitempty"`
	YearFrom int      `json:"year_from,omitempty"`
	YearTo   int      `json:"year_to,omitempty"`
	Genres   []string `json:"genres"`
	People   []string `json:"people"`
	// Words are things the title is about ("robot"), matched against the
	// overview.
	Words []string `json:"words"`
}

const (
	maxRunes  = 300
	maxTokens = 40
	minYear   = 1870
	maxYear   = 2100
)

// genres maps plain words to TMDB genre names.
var genres = map[string]string{
	"action": "Action", "adventure": "Adventure", "adventures": "Adventure",
	"animation": "Animation", "animated": "Animation", "cartoon": "Animation", "cartoons": "Animation", "anime": "Animation",
	"comedy": "Comedy", "comedies": "Comedy", "funny": "Comedy", "hilarious": "Comedy",
	"crime": "Crime", "heist": "Crime", "gangster": "Crime",
	"documentary": "Documentary", "documentaries": "Documentary", "doc": "Documentary", "docs": "Documentary",
	"drama": "Drama", "dramas": "Drama",
	"family": "Family", "kids": "Family",
	"fantasy": "Fantasy", "magic": "Fantasy",
	"history": "History", "historical": "History",
	"horror": "Horror", "scary": "Horror", "spooky": "Horror", "creepy": "Horror",
	"mystery": "Mystery", "detective": "Mystery", "whodunit": "Mystery",
	"romance": "Romance", "romantic": "Romance", "romcom": "Romance",
	"space": "Science Fiction", "sci-fi": "Science Fiction", "scifi": "Science Fiction", "futuristic": "Science Fiction",
	"thriller": "Thriller", "thrillers": "Thriller", "suspense": "Thriller",
	"war": "War", "western": "Western", "westerns": "Western", "cowboy": "Western",
}

// kinds maps plain words to a kind.
var kinds = map[string]string{
	"movie": "movie", "movies": "movie", "film": "movie", "films": "movie",
	"show": "show", "shows": "show", "series": "show", "tv": "show", "sitcom": "show",
	"music": "music", "album": "music", "albums": "music", "band": "music", "artist": "music", "song": "music", "songs": "music",
}

// keywords are things a title is about, never a name on their own.
var keywords = map[string]bool{
	"robot": true, "robots": true, "alien": true, "aliens": true, "dinosaur": true, "dinosaurs": true,
	"zombie": true, "zombies": true, "vampire": true, "vampires": true, "shark": true, "sharks": true,
	"dog": true, "dogs": true, "cat": true, "cats": true, "dragon": true, "dragons": true,
	"ghost": true, "ghosts": true, "pirate": true, "pirates": true, "superhero": true, "superheroes": true,
	"astronaut": true, "astronauts": true, "spaceship": true, "planet": true, "time": true, "travel": true,
	"heist": true, "witch": true, "witches": true, "monster": true, "monsters": true, "ai": true,
}

// filler words carry no meaning in a described search.
var filler = map[string]bool{
	"a": true, "an": true, "the": true, "that": true, "this": true, "some": true, "one": true, "any": true,
	"i": true, "me": true, "my": true, "want": true, "find": true, "looking": true, "look": true, "for": true,
	"of": true, "and": true, "or": true, "is": true, "was": true, "it": true, "its": true, "where": true,
	"there": true, "which": true, "who": true, "set": true, "kind": true, "sort": true, "like": true,
	"good": true, "great": true, "best": true, "old": true, "new": true, "something": true, "watch": true,
	"to": true, "from": true, "in": true, "on": true, "at": true, "around": true, "please": true, "thing": true, "has": true, "had": true, "called": true, "named": true,
}

var decadeWords = map[string]int{
	"twenties": 1920, "thirties": 1930, "forties": 1940, "fifties": 1950, "sixties": 1960,
	"seventies": 1970, "eighties": 1980, "nineties": 1990,
}

// Parse reads text. It never fails and never panics.
func Parse(text string) Query {
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > maxRunes {
		text = strings.TrimSpace(string(r[:maxRunes]))
	}
	q := Query{Text: text, Plain: true, Name: text, Genres: []string{}, People: []string{}, Words: []string{}}
	raw := tokens(text)
	if len(raw) > maxTokens {
		raw = raw[:maxTokens]
	}
	if len(raw) < 2 && !allGenres(raw) {
		return q // one word ("2012", "Se7en", "Alien") is a name
	}
	low := make([]string, len(raw))
	for i, t := range raw {
		low[i] = strings.ToLower(t)
	}
	if !described(low) {
		return q
	}
	q.Plain = false
	var name []string
	used := make([]bool, len(low))
	for i := 0; i < len(low); i++ {
		if used[i] {
			continue
		}
		w := low[i]
		next := ""
		if i+1 < len(low) {
			next = low[i+1]
		}
		switch {
		case w == "show" && next == "me":
			used[i+1] = true
		case w == "science" && next == "fiction":
			used[i+1] = true
			q.addGenre("Science Fiction")
		case kinds[w] != "" && !(w == "music" && q.Kind != ""):
			if q.Kind == "" {
				q.Kind = kinds[w]
			}
		case genres[w] != "":
			q.addGenre(genres[w])
		case decade(w) > 0:
			q.YearFrom, q.YearTo = decade(w), decade(w)+9
		case (w == "from" || w == "in" || w == "of" || w == "around") && year(next) > 0:
			used[i+1] = true
			q.YearFrom, q.YearTo = year(next), year(next)
		case (w == "after" || w == "since") && year(next) > 0:
			used[i+1] = true
			q.YearFrom, q.YearTo = year(next), 0
			if w == "after" {
				q.YearFrom++
			}
		case w == "before" && year(next) > 0:
			used[i+1] = true
			q.YearFrom, q.YearTo = 0, year(next)-1
		case year(w) > 0 && q.YearFrom == 0 && q.YearTo == 0:
			q.YearFrom, q.YearTo = year(w), year(w)
		case w == "with" || w == "starring" || w == "featuring" || w == "about":
			j := i + 1
			var got, gotLow []string
			for ; j < len(low) && len(got) < 4; j++ {
				x := low[j]
				if stop(x) || (len(got) > 0 && filler[x]) {
					break
				}
				used[j] = true
				if filler[x] {
					gotLow = append(gotLow, x)
					continue
				}
				got = append(got, raw[j])
				gotLow = append(gotLow, x)
			}
			q.with(w, got, gotLow)
		case keywords[w]:
			q.addWord(w)
		case filler[w]:
		default:
			name = append(name, raw[i])
		}
	}
	q.Name = strings.Join(name, " ")
	return q
}

// with files the words after "with", "starring" or "about".
func (q *Query) with(lead string, got, gotLow []string) {
	if len(got) == 0 {
		return
	}
	article := len(gotLow) > 0 && filler[gotLow[0]]
	capital := true
	for _, g := range got {
		if r := []rune(g); !unicode.IsUpper(r[0]) {
			capital = false
		}
	}
	person := lead == "starring" || lead == "featuring" ||
		(lead == "with" && !article && (capital || len(got) >= 2) && !keywords[strings.ToLower(got[0])])
	if person {
		q.People = append(q.People, strings.Join(got, " "))
		return
	}
	for _, g := range got {
		g = strings.ToLower(g)
		if gn := genres[g]; gn != "" {
			q.addGenre(gn)
		} else {
			q.addWord(g)
		}
	}
}

func (q *Query) addGenre(g string) {
	for _, x := range q.Genres {
		if x == g {
			return
		}
	}
	q.Genres = append(q.Genres, g)
}

func (q *Query) addWord(w string) {
	for _, x := range q.Words {
		if x == w {
			return
		}
	}
	q.Words = append(q.Words, w)
}

// described says the text describes a title rather than naming it: it has
// a kind word, a year phrase, a decade, "with"/"starring"/"about", or is
// only genre words ("scary comedy").
func described(low []string) bool {
	if allGenres(low) {
		return true
	}
	for i, w := range low {
		next := ""
		if i+1 < len(low) {
			next = low[i+1]
		}
		switch {
		case kinds[w] != "" && !(w == "show" && next == "me"):
			return true
		case decade(w) > 0:
			return true
		case (w == "from" || w == "in" || w == "after" || w == "before" || w == "since" || w == "around") && year(next) > 0:
			return true
		case w == "starring" || w == "featuring" || w == "about":
			return true
		}
	}
	return false
}

func allGenres(low []string) bool {
	if len(low) == 0 {
		return false
	}
	for _, w := range low {
		if genres[strings.ToLower(w)] == "" {
			return false
		}
	}
	return true
}

// stop ends a "with ..." run.
func stop(w string) bool {
	return kinds[w] != "" || decade(w) > 0 || year(w) > 0 || w == "with" || w == "from" || w == "in" ||
		w == "after" || w == "before" || w == "since" || w == "and" || w == "on" || w == "at" || w == "starring" || w == "about" || w == "where" || w == "who"
}

// year reads a plain four-digit year in range, 0 otherwise.
func year(w string) int {
	if len(w) != 4 {
		return 0
	}
	n, err := strconv.Atoi(w)
	if err != nil || n < minYear || n > maxYear {
		return 0
	}
	return n
}

// decade reads "90s", "1990s", "90's" or "nineties" as its first year.
func decade(w string) int {
	if y, ok := decadeWords[w]; ok {
		return y
	}
	w = strings.TrimPrefix(w, "'")
	w = strings.ReplaceAll(w, "'", "")
	if !strings.HasSuffix(w, "s") {
		return 0
	}
	d := strings.TrimSuffix(w, "s")
	n, err := strconv.Atoi(d)
	if err != nil {
		return 0
	}
	switch {
	case len(d) == 2 && n%10 == 0:
		if n >= 30 {
			return 1900 + n
		}
		return 2000 + n
	case len(d) == 4 && n%10 == 0 && n >= minYear && n <= maxYear:
		return n
	}
	return 0
}

// tokens splits text into words; letters, digits, "-" and "'" stay inside.
func tokens(s string) []string {
	return strings.FieldsFunc(s, func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c) && !unicode.IsMark(c) && c != '-' && c != '\''
	})
}
