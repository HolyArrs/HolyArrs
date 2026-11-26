package mediafiles

import "testing"

func TestTrustedTMDB(t *testing.T) {
	cases := []struct {
		name string
		kind string
		ids  map[string]string
		want string
	}{
		{"show with its own tmdb id", "show", map[string]string{"tmdb": "1399", "tvdb": "121361"}, "1399"},
		{"show tmdb copied from tvdb (#342)", "show", map[string]string{"tmdb": "82817", "tvdb": "82817"}, ""},
		{"show tmdb copied from tvmaze", "show", map[string]string{"tmdb": "500", "tvdb": "9", "tvmaze": "500"}, ""},
		{"show without tmdb", "show", map[string]string{"tvdb": "82817"}, ""},
		{"show with only tmdb", "show", map[string]string{"tmdb": "82817"}, "82817"},
		{"show with no ids", "show", nil, ""},
		{"movie keeps a tmdb id equal to another id", "movie", map[string]string{"tmdb": "603", "tvdb": "603"}, "603"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Title{Kind: c.kind, SourceIDs: c.ids}.TrustedTMDB()
			if got != c.want {
				t.Fatalf("TrustedTMDB() = %q, want %q", got, c.want)
			}
		})
	}
}
