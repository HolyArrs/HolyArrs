package musicscore

import "testing"

func TestGenreName_Variants(t *testing.T) {
	for in, want := range map[string]string{
		"alternativerock":  "Alternative Rock",
		"alternative rock": "Alternative Rock",
		"hip hop":          "Hip-Hop",
		"r&b":              "R&B",
		"k-pop":            "K-Pop",
		"synthpop":         "Synth Pop",
		"indie":            "Indie",
		"post-punk":        "Post-Punk",
		"drum and bass":    "Drum and Bass",
		"edm":              "EDM",
		"uk garage":        "UK Garage",
		"metalcore":        "Metalcore",
		"":                 "",
	} {
		if got := GenreName(in); got != want {
			t.Errorf("GenreName(%q) = %q, want %q", in, got, want)
		}
	}
}
