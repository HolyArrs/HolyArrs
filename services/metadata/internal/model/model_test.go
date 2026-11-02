package model

import "testing"

func TestQueryIdentityIncludesAllVariants(t *testing.T) {
	q := Query{Provider: "tmdb", Kind: "movie", Operation: "search", Q: "synthetic", Language: "en", Region: "US", Page: 1, Limit: 20}
	base := q.Key()
	if len(base) != 64 {
		t.Fatalf("key length %d", len(base))
	}
	variants := []Query{q, q, q, q, q}
	variants[0].Language = "es"
	variants[1].Region = "GB"
	variants[2].Page = 2
	variants[3].Kind = "show"
	variants[4].Limit = 10
	for _, v := range variants {
		if v.Key() == base {
			t.Fatalf("query identity collision: %+v", v)
		}
	}
}

func TestQueryValidation(t *testing.T) {
	valid := Query{Provider: "tmdb", Kind: "movie", Operation: "detail", ID: "42", Page: 1, Limit: 20}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Query){func(q *Query) { q.ID = "../config" }, func(q *Query) { q.Provider = "arbitrary" }, func(q *Query) { q.Page = 101 }, func(q *Query) { q.Limit = 101 }, func(q *Query) { q.Operation = "season" }} {
		q := valid
		change(&q)
		if q.Validate() == nil {
			t.Fatalf("accepted invalid query %+v", q)
		}
	}
	q := Query{Provider: "musicbrainz", Kind: "artist", Operation: "search", Q: "synthetic", Page: 1, Limit: 20}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
}
