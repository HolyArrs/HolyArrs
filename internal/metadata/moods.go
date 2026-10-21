package metadata

import (
	"sort"
	"strings"
)

// maxMoods is how many mood words a title carries on its hover card.
const maxMoods = 3

// moodWords maps parts of TMDB keyword names to our own mood words. A
// keyword matches when it contains the part (lower case).
var moodWords = []struct {
	mood  string
	parts []string
}{
	{"Dark", []string{"dark", "bleak", "nihilis", "dystopia", "serial killer", "revenge", "despair", "tragic", "noir"}},
	{"Exciting", []string{"action", "chase", "heist", "explosion", "martial arts", "shootout", "race", "adventure", "battle", "superhero"}},
	{"Feel-good", []string{"feel-good", "feel good", "heartwarming", "friendship", "uplifting", "family", "happy ending", "wholesome"}},
	{"Suspenseful", []string{"suspense", "thriller", "conspiracy", "mystery", "whodunit", "murder", "investigation", "hostage", "kidnapping", "tense"}},
	{"Witty", []string{"witty", "satire", "parody", "sarcas", "dark comedy", "black comedy", "spoof", "absurd", "comedy", "humor"}},
	{"Romantic", []string{"romance", "romantic", "love", "wedding", "falling in love", "relationship"}},
	{"Emotional", []string{"emotional", "grief", "loss", "tearjerker", "illness", "coming of age", "melancholy", "loneliness", "death of"}},
	{"Mind-bending", []string{"time travel", "time loop", "parallel", "alternate reality", "dream", "mind", "twist", "surreal", "simulation", "multiverse", "nonlinear"}},
	{"Cozy", []string{"cozy", "christmas", "holiday", "small town", "baking", "village", "gentle", "slice of life"}},
	{"Gritty", []string{"gritty", "gangster", "drug", "mafia", "crime", "prison", "violence", "corruption", "cartel", "war", "brutal"}},
	{"Inspiring", []string{"inspiring", "inspirational", "biography", "based on true story", "underdog", "triumph", "overcoming"}},
	{"Scary", []string{"horror", "ghost", "haunted", "demon", "monster", "slasher", "zombie", "possession", "supernatural", "creature"}},
}

// genreMoods are the moods a TMDB genre leans to; they only add weight.
var genreMoods = map[string][]string{
	"Action": {"Exciting"}, "Adventure": {"Exciting"}, "Action & Adventure": {"Exciting"},
	"Animation": {"Feel-good"}, "Family": {"Feel-good", "Cozy"}, "Kids": {"Feel-good"},
	"Comedy": {"Witty"}, "Crime": {"Gritty"}, "Documentary": {"Inspiring"}, "Drama": {"Emotional"},
	"Horror": {"Scary"}, "Mystery": {"Suspenseful"}, "Thriller": {"Suspenseful"}, "Romance": {"Romantic"},
	"Science Fiction": {"Mind-bending"}, "Sci-Fi & Fantasy": {"Mind-bending"}, "War": {"Gritty"}, "War & Politics": {"Gritty"},
}

// plainGenres are TMDB genres that carry no mood of their own (a talk
// show, the news, reality TV): a keyword mood on them is not trusted.
var plainGenres = map[string]bool{"Reality": true, "Talk": true, "News": true, "Soap": true}

// topicWords are plain subject words read from keywords; they stand in
// with the genres when no mood fits (WWE Raw: Sports, Reality).
var topicWords = []struct {
	topic string
	parts []string
}{
	{"Sports", []string{"sports", "sport", "wrestling", "football", "basketball", "baseball", "boxing", "soccer", "olympic", "racing"}},
	{"Music", []string{"concert", "musical", "music"}},
	{"Cooking", []string{"cooking", "chef", "baking competition"}},
}

// Moods turns a title's TMDB keywords and genres into up to three words,
// most matched first. A mood needs its genre to lean to it or two of its
// keywords; plain genres (Reality, Talk, News, Soap) take no mood. When
// fewer than two moods fit, topic words and the genres top the list up.
func Moods(keywords, genres []string) []string {
	score := map[string]int{}
	order := map[string]int{}
	for i, m := range moodWords {
		order[m.mood] = i
	}
	lean := map[string]bool{}
	plain := false
	for _, g := range genres {
		if plainGenres[g] {
			plain = true
		}
		for _, m := range genreMoods[g] {
			lean[m] = true
		}
	}
	var topics []string
	for _, k := range keywords {
		k = strings.ToLower(k)
		for _, m := range moodWords {
			for _, p := range m.parts {
				if strings.Contains(" "+k, " "+p) {
					score[m.mood] += 2
					break
				}
			}
		}
		for _, t := range topicWords {
			for _, p := range t.parts {
				if strings.Contains(" "+k, " "+p) && !contains(topics, t.topic) {
					topics = append(topics, t.topic)
					break
				}
			}
		}
	}
	out := []string{}
	if !plain {
		for m, s := range score {
			if lean[m] || s >= 4 {
				out = append(out, m)
			}
		}
		for _, g := range genres {
			for _, m := range genreMoods[g] {
				score[m]++
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if score[out[i]] != score[out[j]] {
				return score[out[i]] > score[out[j]]
			}
			return order[out[i]] < order[out[j]]
		})
	}
	if len(out) >= 2 {
		if len(out) > maxMoods {
			out = out[:maxMoods]
		}
		return out
	}
	for _, w := range append(topics, genres...) {
		if len(out) == maxMoods {
			break
		}
		if !contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func contains(list []string, w string) bool {
	for _, x := range list {
		if x == w {
			return true
		}
	}
	return false
}

// SubMood is one mood row inside a genre: our own label and the TMDB
// keyword names whose titles fill it (any of them).
type SubMood struct {
	Label    string
	Keywords []string
}

// genreSubMoods are a genre page's mood rows, built on the mood words
// above (Scary, Suspenseful, Mind-bending...) narrowed to the genre.
var genreSubMoods = map[string][]SubMood{
	"Horror": {
		{"Supernatural", []string{"supernatural", "ghost", "haunted house", "demon", "possession"}},
		{"Psychological", []string{"psychological thriller", "psychological horror", "paranoia", "madness"}},
		{"Slasher", []string{"slasher", "serial killer", "masked killer"}},
		{"Found footage", []string{"found footage", "mockumentary"}},
		{"Creature features", []string{"monster", "creature", "zombie", "vampire"}},
	},
	"Thriller": {
		{"Conspiracy", []string{"conspiracy", "government conspiracy", "cover-up"}},
		{"Heist", []string{"heist", "bank robbery", "robbery"}},
		{"Psychological", []string{"psychological thriller", "paranoia", "obsession"}},
		{"Whodunit", []string{"whodunit", "murder mystery", "investigation"}},
	},
	"Comedy": {
		{"Satire", []string{"satire", "political satire", "parody", "spoof"}},
		{"Dark comedy", []string{"dark comedy", "black comedy"}},
		{"Romantic comedy", []string{"romantic comedy", "falling in love"}},
		{"Feel-good", []string{"feel-good", "heartwarming", "friendship"}},
	},
	"Drama": {
		{"Based on a true story", []string{"based on true story", "biography"}},
		{"Coming of age", []string{"coming of age", "teenager"}},
		{"Tearjerkers", []string{"tearjerker", "grief", "terminal illness"}},
		{"Family drama", []string{"family relationships", "dysfunctional family"}},
	},
	"Science Fiction": {
		{"Time travel", []string{"time travel", "time loop"}},
		{"Space", []string{"space", "outer space", "spacecraft", "space travel"}},
		{"Dystopia", []string{"dystopia", "post-apocalyptic future"}},
		{"Artificial intelligence", []string{"artificial intelligence (a.i.)", "robot", "android"}},
	},
	"Sci-Fi & Fantasy": {
		{"Time travel", []string{"time travel", "time loop"}},
		{"Space", []string{"space", "outer space", "spacecraft"}},
		{"Dystopia", []string{"dystopia", "post-apocalyptic future"}},
		{"Magic", []string{"magic", "sword and sorcery", "dragon"}},
	},
	"Action": {
		{"Martial arts", []string{"martial arts", "kung fu"}},
		{"Heist", []string{"heist", "bank robbery"}},
		{"Superheroes", []string{"superhero", "based on comic"}},
		{"Revenge", []string{"revenge", "vigilante"}},
	},
	"Action & Adventure": {
		{"Superheroes", []string{"superhero", "based on comic"}},
		{"Martial arts", []string{"martial arts", "kung fu"}},
		{"Revenge", []string{"revenge", "vigilante"}},
	},
	"Crime": {
		{"Gangsters", []string{"gangster", "mafia", "organized crime"}},
		{"Heist", []string{"heist", "bank robbery"}},
		{"Serial killers", []string{"serial killer", "murder investigation"}},
		{"True crime", []string{"true crime", "based on true story"}},
	},
	"Romance": {
		{"Romantic comedy", []string{"romantic comedy"}},
		{"Period romance", []string{"period drama", "historical"}},
		{"Love triangles", []string{"love triangle"}},
	},
	"Fantasy": {
		{"Magic", []string{"magic", "witch", "wizard"}},
		{"Dragons", []string{"dragon", "sword and sorcery"}},
		{"Fairy tales", []string{"fairy tale", "based on fairy tale"}},
	},
	"Animation": {
		{"Anime", []string{"anime"}},
		{"Feel-good", []string{"friendship", "heartwarming"}},
		{"Adult animation", []string{"adult animation"}},
	},
	"Mystery": {
		{"Whodunit", []string{"whodunit", "murder mystery"}},
		{"Detectives", []string{"detective", "investigation"}},
		{"Supernatural", []string{"supernatural", "ghost"}},
	},
	"Documentary": {
		{"True crime", []string{"true crime", "crime"}},
		{"Nature", []string{"nature", "wildlife"}},
		{"Music", []string{"music", "concert"}},
	},
}

// SubMoods are the mood rows of a genre page (by the genre's TMDB name).
func SubMoods(genre string) []SubMood { return genreSubMoods[genre] }
