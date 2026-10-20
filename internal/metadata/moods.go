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
	{"Inspiring", []string{"inspiring", "inspirational", "biography", "based on true story", "underdog", "sports", "triumph", "overcoming"}},
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

// Moods turns a title's TMDB keywords and genres into up to three mood
// words, most matched first. Only keywords pick a mood; genres add weight
// and top the list up. With no keyword match it answers the genres.
func Moods(keywords, genres []string) []string {
	score := map[string]int{}
	order := map[string]int{}
	for i, m := range moodWords {
		order[m.mood] = i
	}
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
	}
	if len(score) == 0 {
		out := []string{}
		for _, g := range genres {
			if len(out) == maxMoods {
				break
			}
			out = append(out, g)
		}
		return out
	}
	for _, g := range genres {
		for _, m := range genreMoods[g] {
			score[m]++
		}
	}
	out := make([]string, 0, len(score))
	for m := range score {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if score[out[i]] != score[out[j]] {
			return score[out[i]] > score[out[j]]
		}
		return order[out[i]] < order[out[j]]
	})
	if len(out) > maxMoods {
		out = out[:maxMoods]
	}
	return out
}
