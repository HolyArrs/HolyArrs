package budget

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

type countFetcher struct{ n int }

func (c *countFetcher) Fetch(context.Context, model.Query) (model.Result, error) {
	c.n++
	return model.Result{Data: []byte(`{}`), Complete: true}, nil
}

func TestBudgetPerProviderAndRefill(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	b := New(map[string]int{"tmdb": 2, "musicbrainz": 1})
	b.Now = func() time.Time { return now }
	inner := &countFetcher{}
	f := Fetcher{Fetcher: inner, Budget: b}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := f.Fetch(ctx, model.Query{Provider: "tmdb"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.Fetch(ctx, model.Query{Provider: "tmdb"})
	var fe *model.FetchError
	if !errors.As(err, &fe) || fe.Code != "upstream_budget" || inner.n != 2 {
		t.Fatalf("spent budget: %v calls %d", err, inner.n)
	}
	// Independent per provider.
	if _, err := f.Fetch(ctx, model.Query{Provider: "musicbrainz"}); err != nil {
		t.Fatal("musicbrainz charged to tmdb")
	}
	// Refills over the hour.
	now = now.Add(30 * time.Minute)
	if _, err := f.Fetch(ctx, model.Query{Provider: "tmdb"}); err != nil {
		t.Fatal("no refill")
	}
	if b.Refused()["tmdb"] != 1 {
		t.Fatalf("refused %v", b.Refused())
	}
}

func TestTransportChargesByHost(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()
	b := New(map[string]int{"listenbrainz": 1, "deezer": 1})
	u, _ := url.Parse(srv.URL)
	// Point the budgeted hosts at the test server.
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme, r2.URL.Host, r2.Host = u.Scheme, u.Host, u.Host
		return http.DefaultTransport.RoundTrip(r2)
	})
	c := &http.Client{Transport: Transport{Base: base, Budget: b}}
	get := func(raw string) error {
		resp, err := c.Get(raw)
		if err == nil {
			resp.Body.Close()
		}
		return err
	}
	if err := get("https://api.listenbrainz.org/1/x"); err != nil {
		t.Fatal(err)
	}
	if err := get("https://api.listenbrainz.org/1/x"); !errors.Is(err, ErrSpent) {
		t.Fatalf("second listenbrainz call: %v", err)
	}
	if err := get("https://api.deezer.com/chart"); err != nil {
		t.Fatal(err)
	}
	if err := get("https://example.org/free"); err != nil {
		t.Fatal("unbudgeted host refused")
	}
	if calls != 3 {
		t.Fatalf("upstream calls %d", calls)
	}
	if Provider("api.themoviedb.org") != "tmdb" || Provider("musicbrainz.org") != "musicbrainz" || Provider("coverartarchive.org") != "" {
		t.Fatal("host map")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLeft(t *testing.T) {
	now := time.Unix(0, 0)
	b := New(map[string]int{"mdblist": 10})
	b.Now = func() time.Time { return now }
	if b.Left("mdblist") != 10 || b.Left("nobody") != -1 {
		t.Fatal(b.Left("mdblist"))
	}
	b.Take("mdblist")
	b.Take("mdblist")
	if b.Left("mdblist") != 8 {
		t.Fatal(b.Left("mdblist"))
	}
	now = now.Add(6 * time.Minute) // a tenth of the hour refills one
	if l := b.Left("mdblist"); l < 8.99 || l > 9.01 {
		t.Fatal(l)
	}
}
