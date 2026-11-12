package requests

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// countingTransport counts what goes through it.
type countingTransport struct{ n atomic.Int32 }

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return http.DefaultTransport.RoundTrip(req)
}

func TestTMDBListsGoTheModesWay(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	base := e.svc.TMDBBase
	via := &countingTransport{}
	e.svc.TMDBBase, e.svc.TMDBToken = "", nil
	e.svc.TMDBRoute = func(context.Context) (string, func(context.Context) (string, error), *http.Client, error) {
		return base, func(context.Context) (string, error) { return fakeTMDBToken, nil }, &http.Client{Transport: via}, nil
	}
	got, err := e.svc.fetchTMDB(ctx, "42", "movie")
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if via.n.Load() != 2 {
		t.Fatalf("the route's client carried %d requests, want 2", via.n.Load())
	}
	before := e.hits.Load()
	e.svc.TMDBRoute = func(context.Context) (string, func(context.Context) (string, error), *http.Client, error) {
		return "", nil, nil, metadata.ErrSourceOff
	}
	if _, err := e.svc.fetchTMDB(ctx, "42", "movie"); !errors.Is(err, ErrListsOff) {
		t.Fatalf("off: %v", err)
	}
	if e.hits.Load() != before {
		t.Fatal("a request went out while lookups are off")
	}
}
