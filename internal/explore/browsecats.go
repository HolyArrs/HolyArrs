package explore

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Search's sectioned Browse: a curated table of categories (genres, moods,
// stories, worlds, eras, languages, collections), each one or two TMDB
// discover queries built only from what the metadata server allows: genre
// ids, keyword names (looked up once to ids), original language and
// release-date ranges.

// BrowseSection is one heading of the Browse grid.
type BrowseSection struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// BrowseSections are the Browse headings, in order.
var BrowseSections = []BrowseSection{
	{"genres", "Genres"}, {"moods", "Moods"}, {"stories", "Stories & themes"}, {"worlds", "Worlds & settings"},
	{"eras", "Eras"}, {"world", "Around the world"}, {"collections", "Collections"},
}

// CatQuery is one kind's discover query for a category. Genres and Without
// are TMDB genre ids ("|" any of, "," all of); Keywords are keyword names
// (any of).
type CatQuery struct {
	Genres, Without string
	Keywords        []string
	Lang            string
	From, To        string
}

// Category is one Browse tile's definition. Movie or Show is nil when the
// category is not for that kind.
type Category struct {
	ID, Section, Name string
	Movie, Show       *CatQuery
	// Kid: the category is kid-safe.
	Kid bool
	// Months limits the category to these months (Holiday), nil: always.
	Months []time.Month
}

// BrowseMin is the fewest titles with a backdrop a category needs to show.
const BrowseMin = 12

func kw(k ...string) []string { return k }

func both(q CatQuery) (*CatQuery, *CatQuery) { m, s := q, q; return &m, &s }

func cat(id, section, name string, kid bool, q CatQuery) Category {
	m, s := both(q)
	return Category{ID: id, Section: section, Name: name, Movie: m, Show: s, Kid: kid}
}

func movieCat(id, section, name string, kid bool, q CatQuery) Category {
	return Category{ID: id, Section: section, Name: name, Movie: &q, Kid: kid}
}

func showCat(id, section, name string, kid bool, q CatQuery) Category {
	return Category{ID: id, Section: section, Name: name, Show: &q, Kid: kid}
}

func split(id, name string, kid bool, movieGenre, showGenre string, showKw []string) Category {
	return Category{ID: id, Section: "genres", Name: name, Kid: kid,
		Movie: &CatQuery{Genres: movieGenre}, Show: &CatQuery{Genres: showGenre, Keywords: showKw}}
}

func years(from, to int) CatQuery {
	q := CatQuery{}
	if from > 0 {
		q.From = strconv.Itoa(from) + "-01-01"
	}
	if to > 0 {
		q.To = strconv.Itoa(to) + "-12-31"
	}
	return q
}

func lang(l string) CatQuery { return CatQuery{Lang: l} }

func kwq(k ...string) CatQuery { return CatQuery{Keywords: k} }

// Categories is the Browse table, broad first within each section.
var Categories = []Category{
	// Genres: TMDB's genres, TV's combined ones split in two.
	split("action", "Action", false, "28", "10759", nil),
	cat("comedy", "genres", "Comedy", true, CatQuery{Genres: "35"}),
	cat("drama", "genres", "Drama", false, CatQuery{Genres: "18"}),
	{ID: "thriller", Section: "genres", Name: "Thriller", Movie: &CatQuery{Genres: "53"}, Show: &CatQuery{Keywords: kw("thriller", "suspense", "psychological thriller")}},
	{ID: "horror", Section: "genres", Name: "Horror", Movie: &CatQuery{Genres: "27"}, Show: &CatQuery{Keywords: kw("horror", "supernatural horror", "ghost", "zombie")}},
	split("scifi", "Sci-Fi", true, "878", "10765", kw("science fiction", "space", "time travel", "alien", "dystopia", "robot")),
	split("fantasy", "Fantasy", true, "14", "10765", kw("magic", "dragon", "sword and sorcery", "witch", "fairy tale")),
	{ID: "romance", Section: "genres", Name: "Romance", Movie: &CatQuery{Genres: "10749"}, Show: &CatQuery{Keywords: kw("romance", "love", "romantic comedy")}},
	cat("crime", "genres", "Crime", false, CatQuery{Genres: "80"}),
	cat("mystery", "genres", "Mystery", false, CatQuery{Genres: "9648"}),
	cat("animation", "genres", "Animation", true, CatQuery{Genres: "16"}),
	cat("documentary", "genres", "Documentary", true, CatQuery{Genres: "99"}),
	cat("family", "genres", "Family", true, CatQuery{Genres: "10751"}),
	split("adventure", "Adventure", true, "12", "10759", kw("adventure", "quest", "treasure hunt", "expedition", "journey")),
	split("war", "War", false, "10752", "10768", kw("war", "world war ii", "soldier", "military")),
	showCat("politics", "genres", "Politics", false, CatQuery{Genres: "10768", Keywords: kw("politics", "president", "election", "political")}),
	cat("western", "genres", "Western", false, CatQuery{Genres: "37"}),
	{ID: "history", Section: "genres", Name: "History", Movie: &CatQuery{Genres: "36"}, Show: &CatQuery{Keywords: kw("history", "historical", "period drama", "historical drama")}},
	movieCat("music", "genres", "Music", true, CatQuery{Genres: "10402"}),
	showCat("kids", "genres", "Kids", true, CatQuery{Genres: "10762"}),
	showCat("reality", "genres", "Reality", false, CatQuery{Genres: "10764"}),
	showCat("sitcoms", "genres", "Sitcoms", true, CatQuery{Genres: "35", Keywords: kw("sitcom")}),
	showCat("medical", "genres", "Medical", false, kwq("medical drama", "hospital", "doctor")),
	showCat("legal", "genres", "Legal", false, kwq("legal drama", "lawyer", "courtroom")),
	showCat("cops", "genres", "Cop procedurals", false, kwq("police procedural", "police", "detective")),
	showCat("teen", "genres", "Teen", false, kwq("teen drama", "high school", "teenager")),
	showCat("soaps", "genres", "Soaps", false, CatQuery{Genres: "10766"}),
	showCat("talk", "genres", "Talk & late night", false, CatQuery{Genres: "10767"}),
	movieCat("spy", "genres", "Spy", false, kwq("spy", "secret agent", "espionage")),
	movieCat("sports", "genres", "Sports", true, kwq("sport", "sports", "boxing", "football (soccer)", "basketball")),
	movieCat("musicals", "genres", "Musicals", true, kwq("musical")),
	movieCat("disaster", "genres", "Disaster", false, kwq("disaster", "natural disaster", "disaster movie")),

	// Moods.
	cat("feel-good", "moods", "Feel-good", true, kwq("feel-good", "heartwarming", "friendship")),
	cat("dark", "moods", "Dark", false, kwq("dark", "nihilism", "bleak")),
	cat("tearjerkers", "moods", "Tearjerkers", false, kwq("tearjerker", "grief", "terminal illness")),
	cat("mind-bending", "moods", "Mind-bending", false, kwq("mind-bending", "twist ending", "nonlinear timeline", "surrealism")),
	cat("edge", "moods", "Edge-of-your-seat", false, kwq("suspenseful", "tense", "intense")),
	cat("cozy", "moods", "Cozy", true, kwq("cozy", "comforting", "wholesome")),
	cat("inspiring", "moods", "Inspiring", true, kwq("inspirational", "inspiring", "underdog")),
	cat("slow-burn", "moods", "Slow burn", false, kwq("slow burn", "melancholy", "contemplative")),
	cat("funny", "moods", "Laugh out loud", true, kwq("hilarious", "slapstick", "absurd")),
	cat("romantic", "moods", "Swoon-worthy", false, kwq("romantic", "falling in love", "love at first sight")),
	cat("scary", "moods", "Scary", false, kwq("scary", "terror", "creepy")),
	cat("thoughtful", "moods", "Thought-provoking", false, kwq("thought-provoking", "philosophical", "existentialism")),
	cat("uplifting", "moods", "Uplifting", true, kwq("uplifting", "hopeful", "optimism")),
	cat("quirky", "moods", "Quirky", true, kwq("quirky", "eccentric", "offbeat")),
	cat("gritty", "moods", "Gritty", false, kwq("gritty", "violence", "neo-noir")),
	cat("epic-feel", "moods", "Epic", true, kwq("epic", "spectacle", "battle")),
	cat("nostalgic", "moods", "Nostalgic", true, kwq("nostalgic", "nostalgia", "childhood")),
	cat("satire", "moods", "Satirical", false, kwq("satire", "political satire", "parody", "spoof")),
	cat("dark-comedy", "moods", "Dark comedy", false, kwq("dark comedy", "black comedy")),
	cat("adrenaline", "moods", "Adrenaline", false, kwq("car chase", "shootout", "explosion", "one man army")),

	// Stories & themes.
	cat("heists", "stories", "Heists", false, kwq("heist", "bank robbery", "robbery")),
	cat("revenge", "stories", "Revenge", false, kwq("revenge", "vigilante")),
	cat("time-travel", "stories", "Time travel", true, kwq("time travel", "time loop")),
	cat("survival", "stories", "Survival", false, kwq("survival", "stranded", "wilderness")),
	cat("road-trips", "stories", "Road trips", true, kwq("road trip", "road movie")),
	cat("prison", "stories", "Prison", false, kwq("prison", "prison escape", "prisoner")),
	cat("small-towns", "stories", "Small towns", true, kwq("small town")),
	cat("con-artists", "stories", "Con artists", false, kwq("con artist", "con man", "scam", "swindle")),
	cat("spies", "stories", "Spies", false, kwq("spy", "espionage", "secret agent", "cia")),
	cat("sports-stories", "stories", "Sports", true, kwq("sport", "sports", "boxing", "baseball", "basketball")),
	cat("disaster-stories", "stories", "Disaster", false, kwq("disaster", "natural disaster", "tsunami", "earthquake")),
	cat("courtroom", "stories", "Courtroom", false, kwq("courtroom", "trial", "lawyer", "legal drama")),
	cat("medical-stories", "stories", "Medical", false, kwq("hospital", "doctor", "medical drama", "nurse")),
	cat("cops-stories", "stories", "Cops", false, kwq("police", "police officer", "police procedural", "cop")),
	cat("teen-stories", "stories", "Teen", true, kwq("high school", "teenager", "teen drama")),
	cat("coming-of-age", "stories", "Coming of age", true, kwq("coming of age")),
	cat("true-story", "stories", "Based on a true story", false, kwq("based on true story", "biography", "historical figure")),
	cat("superheroes", "stories", "Superheroes", true, kwq("superhero", "based on comic", "superhero team")),
	cat("martial-arts", "stories", "Martial arts", false, kwq("martial arts", "kung fu", "karate")),
	cat("serial-killers", "stories", "Serial killers", false, kwq("serial killer", "murder investigation")),
	cat("gangsters", "stories", "Gangsters", false, kwq("gangster", "mafia", "organized crime")),
	cat("whodunits", "stories", "Whodunits", false, kwq("whodunit", "murder mystery")),
	cat("conspiracy", "stories", "Conspiracies", false, kwq("conspiracy", "government conspiracy", "cover-up")),
	cat("aliens", "stories", "Aliens", true, kwq("alien", "alien invasion", "extraterrestrial")),
	cat("ai", "stories", "Robots & AI", true, kwq("artificial intelligence (a.i.)", "robot", "android")),
	cat("zombies", "stories", "Zombies", false, kwq("zombie", "zombie apocalypse")),
	cat("vampires", "stories", "Vampires", false, kwq("vampire")),
	cat("monsters", "stories", "Monsters", false, kwq("monster", "creature", "giant monster", "kaiju")),
	cat("magic", "stories", "Magic", true, kwq("magic", "wizard", "witch")),
	cat("dragons", "stories", "Dragons", true, kwq("dragon")),
	cat("fairy-tales", "stories", "Fairy tales", true, kwq("fairy tale", "based on fairy tale")),
	cat("animals", "stories", "Animals", true, kwq("animal", "dog", "cat", "talking animal")),
	cat("friendship", "stories", "Friendship", true, kwq("friendship", "best friend")),
	cat("family-stories", "stories", "Family ties", true, kwq("family relationships", "father son relationship", "mother daughter relationship")),
	cat("love-triangles", "stories", "Love triangles", false, kwq("love triangle")),
	cat("weddings", "stories", "Weddings", false, kwq("wedding", "marriage")),
	cat("music-stories", "stories", "Music & bands", true, kwq("musician", "rock band", "singer")),
	cat("cooking", "stories", "Food & cooking", true, kwq("cooking", "chef", "restaurant", "food")),
	cat("hackers", "stories", "Hackers", false, kwq("hacker", "computer", "cyber")),
	cat("pirates", "stories", "Pirates", true, kwq("pirate")),
	cat("treasure", "stories", "Treasure hunts", true, kwq("treasure hunt", "treasure")),
	cat("assassins", "stories", "Assassins", false, kwq("assassin", "hitman", "contract killer")),
	cat("rise-fall", "stories", "Rise and fall", false, kwq("rise and fall", "drug dealer", "drug cartel")),

	// Worlds & settings.
	cat("space", "worlds", "Space", true, kwq("space", "outer space", "spacecraft", "space travel")),
	cat("dystopia", "worlds", "Dystopia", false, kwq("dystopia", "totalitarianism")),
	cat("post-apocalyptic", "worlds", "Post-apocalyptic", false, kwq("post-apocalyptic future", "apocalypse", "end of the world")),
	cat("medieval", "worlds", "Medieval", false, kwq("medieval", "middle ages", "knight", "castle")),
	cat("cyberpunk", "worlds", "Cyberpunk", false, kwq("cyberpunk", "virtual reality")),
	cat("wild-west", "worlds", "Wild West", false, kwq("wild west", "cowboy", "gunslinger")),
	cat("underwater", "worlds", "Underwater", true, kwq("underwater", "ocean", "submarine", "sea")),
	cat("haunted", "worlds", "Haunted houses", false, kwq("haunted house", "haunting")),
	cat("islands", "worlds", "Islands", true, kwq("island", "deserted island")),
	cat("big-city", "worlds", "Big city", false, kwq("new york city", "los angeles, california", "london, england", "tokyo, japan")),
	cat("high-school", "worlds", "High school", true, kwq("high school", "school")),
	cat("wilderness", "worlds", "Wilderness", true, kwq("wilderness", "forest", "mountain", "jungle")),
	cat("desert", "worlds", "Desert", false, kwq("desert")),
	cat("snow", "worlds", "Snow and ice", true, kwq("snow", "arctic", "winter", "antarctica")),
	cat("ancient", "worlds", "Ancient worlds", false, kwq("ancient rome", "ancient greece", "ancient egypt", "gladiator")),
	cat("victorian", "worlds", "Victorian era", false, kwq("victorian england", "19th century", "period drama")),
	cat("ww2", "worlds", "World War II", false, kwq("world war ii", "nazi", "holocaust")),
	cat("cold-war", "worlds", "Cold War", false, kwq("cold war", "soviet union", "kgb")),
	cat("suburbs", "worlds", "Suburbia", true, kwq("suburbia", "suburb", "neighbor")),
	cat("hotels", "worlds", "Hotels", false, kwq("hotel", "resort")),
	cat("trains-planes", "worlds", "Trains and planes", true, kwq("train", "airplane", "plane crash")),
	cat("hospitals", "worlds", "Hospitals", false, kwq("hospital")),
	cat("alt-worlds", "worlds", "Parallel worlds", true, kwq("parallel world", "alternate reality", "multiverse", "alternate history")),
	cat("fantasy-worlds", "worlds", "Fantasy worlds", true, kwq("fantasy world", "sword and sorcery", "epic fantasy")),
	cat("jungle", "worlds", "Jungle", true, kwq("jungle", "rainforest", "amazon rainforest")),

	// Eras (release years).
	cat("classics", "eras", "Classics before 1970", false, years(0, 1969)),
	cat("70s", "eras", "70s", false, years(1970, 1979)),
	cat("80s", "eras", "80s", false, years(1980, 1989)),
	cat("90s", "eras", "90s", false, years(1990, 1999)),
	cat("2000s", "eras", "2000s", false, years(2000, 2009)),
	cat("2010s", "eras", "2010s", false, years(2010, 2019)),

	// Around the world (original language).
	cat("korean", "world", "Korean", false, lang("ko")),
	cat("japanese", "world", "Japanese", false, lang("ja")),
	cat("spanish", "world", "Spanish", false, lang("es")),
	cat("french", "world", "French", false, lang("fr")),
	cat("hindi", "world", "Bollywood & Hindi", false, lang("hi")),
	cat("nordic", "world", "Nordic noir", false, CatQuery{Lang: "sv", Genres: "80|9648|53"}),
	cat("danish", "world", "Danish", false, lang("da")),
	cat("british", "world", "British", false, CatQuery{Lang: "en", Keywords: kw("london, england", "england", "british", "scotland")}),
	cat("italian", "world", "Italian", false, lang("it")),
	cat("german", "world", "German", false, lang("de")),
	cat("latin", "world", "Latin American", false, CatQuery{Lang: "es", Keywords: kw("mexico", "brazil", "argentina", "colombia")}),
	cat("portuguese", "world", "Portuguese", false, lang("pt")),
	cat("chinese", "world", "Chinese", false, lang("zh")),
	cat("turkish", "world", "Turkish", false, lang("tr")),

	// Collections.
	cat("books", "collections", "Based on a book", true, kwq("based on novel or book", "based on novel", "based on young adult novel")),
	movieCat("awards", "collections", "Award winners", false, kwq("oscar (award)", "academy award winner", "golden globe winner", "palme d'or")),
	cat("comics", "collections", "Based on comics", true, kwq("based on comic", "based on manga", "based on graphic novel")),
	showCat("limited", "collections", "Limited series", false, kwq("miniseries", "limited series")),
	cat("anime", "collections", "Anime", true, CatQuery{Lang: "ja", Genres: "16"}),
	cat("remakes", "collections", "Remakes", false, kwq("remake")),
	cat("video-games", "collections", "Based on a video game", true, kwq("based on video game")),
	{ID: "holiday", Section: "collections", Name: "Holiday", Kid: true, Months: []time.Month{time.November, time.December},
		Movie: &CatQuery{Keywords: kw("christmas", "holiday", "thanksgiving", "santa claus")},
		Show:  &CatQuery{Keywords: kw("christmas", "holiday", "santa claus")}},
}

// kidWithout are genres a Kid's category queries always leave out (TMDB
// ids valid for both kinds).
const kidWithout = "27|80|53|10752|10768"

// CategoryByID finds a category.
func CategoryByID(id string) (Category, bool) {
	for _, c := range Categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}

// InSeason says the category shows this month.
func (c Category) InSeason(now time.Time) bool {
	if len(c.Months) == 0 {
		return true
	}
	for _, m := range c.Months {
		if now.Month() == m {
			return true
		}
	}
	return false
}

// Query is the category's query for kind, nil when it has none.
func (c Category) Query(kind string) *CatQuery {
	if kind == "show" {
		return c.Show
	}
	return c.Movie
}

// SectionCategories are the categories of section for the tab sw: those
// with a query for one of the tab's kinds, in season, kid-safe for a Kid;
// under All a name shows once (the first category with it). A Kid never
// gets a category whose query asks for a genre a Kid can't see.
func SectionCategories(section, sw string, kid bool, now time.Time) []Category {
	kinds := Kinds(sw)
	var out []Category
	named := map[string]bool{}
	for _, c := range Categories {
		if c.Section != section || !c.InSeason(now) || (kid && !c.Kid) || named[c.Name] {
			continue
		}
		has := false
		for _, k := range kinds {
			if q := c.Query(k); q != nil && !(kid && kidBlocked(q.Genres)) {
				has = true
			}
		}
		if !has {
			continue
		}
		named[c.Name] = true
		out = append(out, c)
	}
	return out
}

func kidBlocked(genres string) bool {
	for _, g := range strings.FieldsFunc(genres, func(r rune) bool { return r == '|' || r == ',' }) {
		if strings.Contains("|"+kidWithout+"|", "|"+g+"|") {
			return true
		}
	}
	return false
}

// Requests are the discover requests that fill category c on tab sw (one
// per kind). A Kid's leave out kidWithout and, for movies, anything above
// US PG.
func (c Category) Requests(sw string, kid bool) []Request {
	var out []Request
	for _, k := range Kinds(sw) {
		q := c.Query(k)
		if q == nil || (kid && kidBlocked(q.Genres)) {
			continue
		}
		r := Request{List: "discover", Kind: k, Genres: q.Genres, Without: q.Without, Keywords: strings.Join(q.Keywords, "|"),
			Lang: q.Lang, From: q.From, To: q.To, MinVotes: 20}
		if kid {
			r.Without = kidWithout
			if k == "movie" {
				r.Cert = "PG"
			}
		}
		out = append(out, r)
	}
	return out
}

// CategoryTile is a category's tile in a section.
type CategoryTile struct {
	BrowseTile
	Category string `json:"category"`
	Section  string `json:"section"`
	Count    int    `json:"count"`
}

// CategoryResult is a category's fetched titles (both kinds merged).
type CategoryResult struct {
	Cat   Category
	Items []Item
}

// PickCategoryTiles turns fetched categories into tiles: a category shows
// only with at least BrowseMin titles that have a backdrop, its art the
// most popular such title whose backdrop no tile used (used is shared
// across sections and updated). Order: categories matching the household's
// taste (genre names) first, then by popularity. skip holds names already
// shown (the page's genre tiles).
func PickCategoryTiles(res []CategoryResult, taste []string, used, skip map[string]bool) []CategoryTile {
	likes := map[string]bool{}
	for _, t := range taste {
		likes[t] = true
	}
	type scored struct {
		r            CategoryResult
		liked, pop   float64
		withBackdrop []Item
	}
	var list []scored
	for _, r := range res {
		if skip[r.Cat.Name] {
			continue
		}
		var bd []Item
		for _, it := range r.Items {
			if it.Backdrop != "" {
				bd = append(bd, it)
			}
		}
		if len(bd) < BrowseMin {
			continue
		}
		sort.SliceStable(bd, func(i, j int) bool { return bd[i].Pop > bd[j].Pop })
		s := scored{r: r, withBackdrop: bd}
		for i, it := range bd {
			if i < BrowseMin {
				s.pop += it.Pop
			}
			for _, g := range it.Genres {
				if likes[g] {
					s.liked++
				}
			}
			if it.Owned || it.HH > 0 {
				s.liked += 2
			}
		}
		list = append(list, s)
	}
	sort.SliceStable(list, func(i, j int) bool {
		li, lj := list[i].liked/float64(len(list[i].withBackdrop)), list[j].liked/float64(len(list[j].withBackdrop))
		if (li >= 0.5) != (lj >= 0.5) {
			return li >= 0.5
		}
		return list[i].pop > list[j].pop
	})
	out := []CategoryTile{}
	for _, s := range list {
		for _, it := range s.withBackdrop {
			if used[it.Backdrop] {
				continue
			}
			used[it.Backdrop] = true
			c := s.r.Cat
			out = append(out, CategoryTile{BrowseTile: BrowseTile{Kind: it.Kind, GenreName: c.Name, Name: c.Name, Backdrop: it.Backdrop, From: it.Key()},
				Category: c.ID, Section: c.Section, Count: len(s.r.Items)})
			break
		}
	}
	return out
}

// combinedShowGenres are TV genres the Browse splits in two (their tiles
// come from categories instead).
var combinedShowGenres = map[string]bool{"Action & Adventure": true, "Sci-Fi & Fantasy": true, "War & Politics": true}

// browseNames are display names for TMDB genre names.
var browseNames = map[string]string{"Science Fiction": "Sci-Fi"}

// PageGenreTiles are the Genres section's fast tiles, straight from the
// member's page (no outside call): each genre of the tab that has at least
// BrowseMin titles with a backdrop on the page. TV's combined genres are
// left to their split categories. used gets each tile's backdrop.
func PageGenreTiles(pg Page, sw string, kid bool, used map[string]bool) []BrowseTile {
	count := map[string]int{}
	seen := map[string]bool{}
	kinds := Kinds(sw)
	for _, rows := range [][]Row{pg.Head, pg.Tail} {
		for _, r := range rows {
			for _, it := range r.Items {
				if it.Backdrop == "" || !kindIn(kinds, it.Kind) || seen[it.Key()] {
					continue
				}
				seen[it.Key()] = true
				for _, g0 := range it.Genres {
					_, names := mergeGenre(sw, it.Kind, g0)
					for _, g := range names {
						count[g]++
					}
				}
			}
		}
	}
	out := []BrowseTile{}
	for _, t := range BrowseTiles(pg, sw, kid, nil) {
		if combinedShowGenres[t.GenreName] || count[t.GenreName] < BrowseMin {
			continue
		}
		if n := browseNames[t.GenreName]; n != "" {
			t.Name = n
		}
		used[t.Backdrop] = true
		out = append(out, t)
	}
	return out
}
