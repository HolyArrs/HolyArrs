package metadata

import (
	"slices"
	"testing"
)

func TestContentNotes(t *testing.T) {
	cases := []struct {
		name     string
		keywords []string
		want     []string
	}{
		{"none", nil, nil},
		{"plain keywords", []string{"friendship", "small town", "baking"}, nil},
		{"nudity kinds fold to one", []string{"female nudity", "male nudity", "full frontal nudity"}, []string{"nudity"}},
		{"sex scene", []string{"sex scene"}, []string{"sex"}},
		{"sexual violence", []string{"rape", "sexual abuse"}, []string{"sexual_violence"}},
		{"grape is not rape", []string{"grape", "vineyard"}, nil},
		{"pet dies", []string{"death of a dog", "dog"}, []string{"animal_death"}},
		{"animal cruelty", []string{"animal cruelty"}, []string{"animal_death"}},
		{"suicide", []string{"suicide attempt"}, []string{"suicide"}},
		{"self-harm", []string{"self-harm"}, []string{"suicide"}},
		{"suicide squad is a team", []string{"suicide squad", "suicide mission"}, nil},
		{"heroine is not heroin", []string{"heroine", "female heroine"}, nil},
		{"al gore is a person", []string{"al gore"}, nil},
		{"pedophilia", []string{"pedophilia"}, []string{"child_abuse"}},
		{"drug use", []string{"drug addiction", "heroin"}, []string{"drugs"}},
		{"gore", []string{"gore", "torture"}, []string{"gore"}},
		{"child abuse", []string{"child abuse"}, []string{"child_abuse"}},
		{"domestic violence", []string{"domestic violence"}, []string{"domestic_abuse"}},
		{"upper case", []string{"Nudity", "Drug Use"}, []string{"nudity", "drugs"}},
		{"fixed order, no repeats", []string{"heroin", "nudity", "suicide", "nudity", "gore"}, []string{"nudity", "gore", "suicide", "drugs"}},
		{"blank and spaces", []string{"", "   ", " nudity "}, []string{"nudity"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ContentNotes(c.keywords); !slices.Equal(got, c.want) {
				t.Fatalf("ContentNotes(%q) = %q, want %q", c.keywords, got, c.want)
			}
		})
	}
}

func TestContentNotesKeysAreKnown(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range contentNoteWords {
		if seen[n.note] {
			t.Fatalf("note %q listed twice", n.note)
		}
		seen[n.note] = true
	}
	if len(seen) != len(ContentNoteKeys()) {
		t.Fatalf("ContentNoteKeys has %d, list has %d", len(ContentNoteKeys()), len(seen))
	}
}
