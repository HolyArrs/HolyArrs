package downloads

import "testing"

func TestPickCategory(t *testing.T) {
	cases := []struct {
		name string
		kind string
		have []string
		want string
	}{
		{"none", "show", nil, ""},
		{"sonarr default", "show", []string{"movies", "tv-sonarr"}, "tv-sonarr"},
		{"plain tv", "show", []string{"TV"}, "TV"},
		{"series accent", "show", []string{"Séries"}, "Séries"},
		{"order wins", "show", []string{"sonarr", "tv"}, "tv"},
		{"no partial", "show", []string{"tv-old", "television"}, ""},
		{"spaces trimmed", "show", []string{" shows "}, " shows "},
		{"radarr", "movie", []string{"radarr"}, "radarr"},
		{"movie singular", "movie", []string{"Movie"}, "Movie"},
		{"movies over radarr", "movie", []string{"radarr", "movies"}, "movies"},
		{"music", "artist", []string{"Lidarr"}, "Lidarr"},
		{"music plain", "artist", []string{"music", "lidarr"}, "music"},
		{"other kind", "book", []string{"books"}, ""},
		{"star skipped", "movie", []string{"*"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PickCategory(c.kind, c.have); got != c.want {
				t.Fatalf("PickCategory(%q, %q) = %q, want %q", c.kind, c.have, got, c.want)
			}
		})
	}
}
