package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTMDBReviewsNewestFirstPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/7/reviews" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"page":1,"total_pages":2,"results":[{"author":"old","author_details":{"rating":null},"content":"first","created_at":"2020-01-01T00:00:00Z"}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"page":2,"total_pages":2,"results":[
				{"author":"a","author_details":{"name":"Ann","rating":8},"content":"<b>Great</b> film\r\n<script>x</script>","created_at":"2024-01-02T00:00:00Z"},
				{"author":"b","author_details":{"rating":0},"content":"newer","created_at":"2024-05-01T00:00:00Z"}]}`))
		}
	}))
	defer srv.Close()
	tm := &TMDB{BaseURL: srv.URL, Access: func(context.Context) (string, error) { return "x", nil }, Gap: time.Millisecond}
	p, err := tm.Reviews(context.Background(), "movie", "7", 1, ReviewsNewest)
	if err != nil {
		t.Fatal(err)
	}
	if p.Pages != 2 || len(p.Reviews) != 2 || p.Reviews[0].Author != "b" || p.Reviews[0].Rating != 0 {
		t.Fatalf("page 1 %+v", p)
	}
	if r := p.Reviews[1]; r.Author != "Ann" || r.Rating != 8 || r.Text != "Great film\nx" || r.Date != "2024-01-02" {
		t.Fatalf("review %+v", r)
	}
	p, _ = tm.Reviews(context.Background(), "movie", "7", 2, ReviewsNewest)
	if len(p.Reviews) != 1 || p.Reviews[0].Author != "old" {
		t.Fatalf("page 2 %+v", p)
	}
	p, _ = tm.Reviews(context.Background(), "movie", "7", 3, ReviewsNewest)
	if len(p.Reviews) != 0 {
		t.Fatalf("past the end %+v", p)
	}
	if _, err := tm.Reviews(context.Background(), "artist", "7", 1, ""); err != ErrNotFound {
		t.Fatalf("artist: %v", err)
	}
}

func TestTraktReviews(t *testing.T) {
	var sawKey, sawToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey, sawToken = r.Header.Get("trakt-api-key"), r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/search/tmdb/7":
			_, _ = w.Write([]byte(`[{"type":"show","show":{"ids":{"trakt":55}}}]`))
		case "/shows/55/ratings":
			_, _ = w.Write([]byte(`{"rating":8.1234,"votes":900,"distribution":{"1":1,"10":50}}`))
		case "/shows/55/watching":
			_, _ = w.Write([]byte(`[{"username":"a"},{"username":"b"}]`))
		case "/shows/55/comments/likes":
			w.Header().Set("X-Pagination-Page-Count", "4")
			_, _ = w.Write([]byte(`[{"comment":"Loved <i>it</i>","spoiler":true,"created_at":"2025-03-04T00:00:00Z","likes":12,"user_rating":9,"user":{"username":"zed"}}]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	k := &Trakt{Base: srv.URL, ClientID: "cid", Token: "tok"}
	p, err := k.Reviews(context.Background(), "show", "7", 1, ReviewsHelpful)
	if err != nil {
		t.Fatal(err)
	}
	if sawKey != "cid" || sawToken != "Bearer tok" {
		t.Fatalf("headers %q %q", sawKey, sawToken)
	}
	if p.Score != 8.1 || p.Votes != 900 || p.Watching == nil || *p.Watching != 2 || p.Pages != 4 || len(p.Distribution) != 10 || p.Distribution[9] != 50 || p.Distribution[0] != 1 {
		t.Fatalf("page %+v", p)
	}
	if r := p.Reviews[0]; r.Author != "zed" || r.Text != "Loved it" || !r.Spoiler || r.Likes != 12 || r.Rating != 9 {
		t.Fatalf("comment %+v", r)
	}
	locked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer locked.Close()
	if _, err := (&Trakt{Base: locked.URL, ClientID: "c2", Token: "t2"}).Reviews(context.Background(), "movie", "7", 1, ""); err != ErrTraktLocked {
		t.Fatalf("locked: %v", err)
	}
}
