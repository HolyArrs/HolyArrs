package metadata

import "strings"

// contentNoteWords maps parts of TMDB keyword names to content notes: what
// a title shows that some people want to know before they ask for it. A
// keyword matches when it holds the part as whole words (lower case), so
// "heroine" is not "heroin". The list order is the order the page shows
// the notes in.
var contentNoteWords = []struct {
	note  string
	parts []string
}{
	{"nudity", []string{"nudity", "naked", "nude"}},
	{"sex", []string{"sex scene", "explicit sex", "sexual content", "softcore", "erotic"}},
	{"sexual_violence", []string{"rape", "sexual assault", "sexual abuse", "sexual violence"}},
	{"gore", []string{"gore", "graphic violence", "torture", "splatter", "dismemberment"}},
	{"animal_death", []string{"animal death", "animal cruelty", "animal abuse", "death of a dog", "death of a pet", "dog dies", "dead dog", "death of a cat", "killing an animal"}},
	{"suicide", []string{"suicide", "self-harm", "self harm", "self mutilation"}},
	{"drugs", []string{"drug use", "drug addiction", "drug abuse", "heroin", "cocaine", "overdose", "drug addict"}},
	{"child_abuse", []string{"child abuse", "child molestation", "pedophilia", "pedophile"}},
	{"domestic_abuse", []string{"domestic violence", "domestic abuse", "spousal abuse"}},
}

// contentNoteSkips are keywords that look like a note but name something
// else (a team, a mission or a person).
var contentNoteSkips = map[string]bool{"suicide squad": true, "suicide mission": true, "al gore": true}

// ContentNoteKeys are every note ContentNotes can give, in page order.
func ContentNoteKeys() []string {
	out := make([]string, len(contentNoteWords))
	for i, n := range contentNoteWords {
		out[i] = n.note
	}
	return out
}

// ContentNotes reads a title's TMDB keywords and returns the content notes
// they point to, each once, in page order; nil when none fit.
func ContentNotes(keywords []string) []string {
	hit := map[string]bool{}
	for _, k := range keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" || contentNoteSkips[k] {
			continue
		}
		for _, n := range contentNoteWords {
			for _, p := range n.parts {
				if strings.Contains(" "+k+" ", " "+p+" ") {
					hit[n.note] = true
					break
				}
			}
		}
	}
	var out []string
	for _, n := range contentNoteWords {
		if hit[n.note] {
			out = append(out, n.note)
		}
	}
	return out
}
