package metadata

import "strings"

// The US age-rating ladders for movies and shows decide whether a title's
// rating fits under a person's limit. A limit is one value from either
// ladder; it is mapped onto the other ladder with a fixed table and compared
// by position. No limit ("") allows everything. With a limit set, a title
// without a rating, with a rating that is not on the ladder for its kind, and
// every music artist are not allowed.

// MovieRatings is the US movie ladder, lowest first.
var MovieRatings = []string{"G", "PG", "PG-13", "R", "NC-17"}

// TVRatings is the US TV ladder, lowest first.
var TVRatings = []string{"TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA"}

// movieToTV maps a movie limit onto the TV ladder.
var movieToTV = map[string]string{"G": "TV-G", "PG": "TV-PG", "PG-13": "TV-14", "R": "TV-MA", "NC-17": "TV-MA"}

// tvToMovie maps a TV limit onto the movie ladder.
var tvToMovie = map[string]string{"TV-Y": "G", "TV-Y7": "G", "TV-G": "G", "TV-PG": "PG", "TV-14": "PG-13", "TV-MA": "R"}

// NormRating trims a rating and upper-cases it, so "pg-13 " reads as "PG-13".
func NormRating(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func pos(ladder []string, s string) int {
	for i, v := range ladder {
		if v == s {
			return i
		}
	}
	return -1
}

// ValidRating says max is no limit ("") or a value on either ladder.
func ValidRating(max string) bool {
	m := NormRating(max)
	return m == "" || pos(MovieRatings, m) >= 0 || pos(TVRatings, m) >= 0
}

// ValidRatingCountry says c is a country whose ratings are known; only the US is,
// because titles keep only the US rating.
func ValidRatingCountry(c string) bool { return NormRating(c) == "US" }

// RatingAllows says a title of kind (movie, show or artist) rated rating fits
// under max.
func RatingAllows(max, kind, rating string) bool {
	m := NormRating(max)
	if m == "" {
		return true
	}
	if !ValidRating(m) {
		return false
	}
	r := NormRating(rating)
	var ladder []string
	var limit string
	switch kind {
	case "movie":
		ladder, limit = MovieRatings, m
		if v, ok := tvToMovie[m]; ok {
			limit = v
		}
	case "show":
		ladder, limit = TVRatings, m
		if v, ok := movieToTV[m]; ok {
			limit = v
		}
	default:
		return false
	}
	at := pos(ladder, r)
	return at >= 0 && at <= pos(ladder, limit)
}

// RatingUp says the limit in plain words, such as "up to PG".
func RatingUp(max string) string {
	m := NormRating(max)
	if m == "" {
		return "any rating"
	}
	return "up to " + m
}

// RatingChoices lists every value a limit may take, for error messages.
func RatingChoices() string {
	return strings.Join(append(append([]string{}, MovieRatings...), TVRatings...), ", ")
}
