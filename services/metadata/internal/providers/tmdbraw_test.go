package providers

import (
	"net/url"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

func TestTMDBReadURLAndRawAnswer(t *testing.T) {
	base, _ := url.Parse("https://api.themoviedb.org/3")
	q := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: "/trending/tv/week", Params: "language=en-US", Page: 1, Limit: 20}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	if u := requestURL(base, q).String(); u != "https://api.themoviedb.org/3/trending/tv/week?language=en-US" {
		t.Fatal(u)
	}
	r, err := normalize(q, []byte(`{ "results": [ {"id": 1} ] }`))
	if err != nil || string(r.Data) != `{"results":[{"id":1}]}` || !r.Complete {
		t.Fatalf("%s %v", r.Data, err)
	}
	if _, err := normalize(q, []byte(`[1]`)); err == nil {
		t.Fatal("array accepted")
	}
	bad := q
	bad.Params = "api_key=x"
	if bad.Validate() == nil {
		t.Fatal("unlisted parameter accepted")
	}
}

func TestPersonPopularDropsAdult(t *testing.T) {
	q := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: "/person/popular", Page: 1, Limit: 20}
	res, err := normalize(q, []byte(`{"page":1,"results":[{"id":1,"adult":false},{"id":2,"adult":true},{"id":3}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(res.Data); strings.Contains(s, `"id":2`) || !strings.Contains(s, `"id":1`) || !strings.Contains(s, `"id":3`) {
		t.Fatalf("got %s", s)
	}
}
