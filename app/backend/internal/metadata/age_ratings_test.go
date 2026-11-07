package metadata

import "testing"

func TestAllows(t *testing.T) {
	cases := []struct {
		max, kind, rating string
		want              bool
	}{
		{"", "movie", "", true},
		{"", "artist", "", true},
		{"", "movie", "NC-17", true},
		{"PG", "movie", "G", true},
		{"PG", "movie", "PG", true},
		{"PG", "movie", "PG-13", false},
		{"PG", "show", "TV-PG", true},
		{"PG", "show", "TV-14", false},
		{"TV-Y", "show", "TV-Y7", false},
		{"TV-Y", "show", "TV-Y", true},
		{"TV-Y", "movie", "G", true},
		{"TV-Y", "movie", "PG", false},
		{"PG", "movie", "", false},
		{"PG", "movie", "NR", false},
		{"PG", "movie", "TV-G", false},
		{"PG", "show", "PG", false},
		{"PG", "show", "", false},
		{"R", "artist", "", false},
		{"NC-17", "artist", "", false},
		{"pg-13", "movie", "pg-13", true},
		{" PG-13 ", "movie", " pg-13", true},
		{"G", "show", "TV-G", true},
		{"G", "show", "TV-PG", false},
		{"PG-13", "show", "TV-14", true},
		{"PG-13", "show", "TV-MA", false},
		{"R", "show", "TV-MA", true},
		{"NC-17", "show", "TV-MA", true},
		{"NC-17", "movie", "NC-17", true},
		{"R", "movie", "NC-17", false},
		{"TV-Y7", "movie", "G", true},
		{"TV-G", "movie", "PG", false},
		{"TV-PG", "movie", "PG", true},
		{"TV-PG", "movie", "PG-13", false},
		{"TV-14", "movie", "PG-13", true},
		{"TV-14", "movie", "R", false},
		{"TV-MA", "movie", "R", true},
		{"TV-MA", "movie", "NC-17", false},
		{"TV-14", "show", "TV-14", true},
		{"TV-14", "show", "TV-MA", false},
		{"X", "movie", "G", false},
		{"PG", "episode", "G", false},
	}
	for _, c := range cases {
		if got := RatingAllows(c.max, c.kind, c.rating); got != c.want {
			t.Errorf("Allows(%q, %q, %q) = %v, want %v", c.max, c.kind, c.rating, got, c.want)
		}
	}
}

func TestValid(t *testing.T) {
	for _, ok := range []string{"", "G", "PG", "PG-13", "R", "NC-17", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA", "pg-13", " tv-ma "} {
		if !ValidRating(ok) {
			t.Errorf("Valid(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"X", "NR", "PG13", "TV-PG-13", "12A"} {
		if ValidRating(bad) {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
}

func TestNorm(t *testing.T) {
	for in, want := range map[string]string{"": "", " pg-13 ": "PG-13", "tv-ma": "TV-MA", "G": "G"} {
		if got := NormRating(in); got != want {
			t.Errorf("Norm(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidCountry(t *testing.T) {
	for _, ok := range []string{"US", "us ", " us"} {
		if !ValidRatingCountry(ok) {
			t.Errorf("ValidRatingCountry(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"GB", "", "USA", "DE"} {
		if ValidRatingCountry(bad) {
			t.Errorf("ValidRatingCountry(%q) = true, want false", bad)
		}
	}
}

func TestUp(t *testing.T) {
	for in, want := range map[string]string{"PG": "up to PG", "tv-14": "up to TV-14", "": "any rating"} {
		if got := RatingUp(in); got != want {
			t.Errorf("Up(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChoices(t *testing.T) {
	if got := RatingChoices(); got != "G, PG, PG-13, R, NC-17, TV-Y, TV-Y7, TV-G, TV-PG, TV-14, TV-MA" {
		t.Errorf("RatingChoices() = %q", got)
	}
}
