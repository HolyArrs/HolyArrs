package migrate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowList serves a list in pieces, pause apart, then optionally stalls.
func slowList(t *testing.T, pieces int, pause, stall time.Duration) *apiClient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		_, _ = w.Write([]byte("["))
		for i := 0; i < pieces; i++ {
			if i > 0 {
				_, _ = w.Write([]byte(","))
			}
			_, _ = w.Write([]byte(`{"id":1,"title":"S","path":"/tv/S"}`))
			f.Flush()
			time.Sleep(pause)
		}
		if stall > 0 {
			select {
			case <-time.After(stall):
			case <-r.Context().Done():
			}
			return
		}
		_, _ = w.Write([]byte("]"))
	}))
	t.Cleanup(srv.Close)
	c, err := newAPIClient(Source{Kind: Sonarr, URL: srv.URL, AllowLocal: true, Limits: Limits{Timeout: 300 * time.Millisecond}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestListSlowButSteady: a big list that keeps arriving is read in full even
// when the whole answer takes longer than one call's Timeout.
func TestListSlowButSteady(t *testing.T) {
	c := slowList(t, 12, 100*time.Millisecond, 0)
	ts, err := c.getTitles(context.Background(), "/x")
	if err != nil || len(ts) != 12 {
		t.Fatalf("got %d titles, err %v; want 12", len(ts), err)
	}
}

// TestListStalled: an answer that stops arriving fails after Timeout of silence.
func TestListStalled(t *testing.T) {
	c := slowList(t, 2, 0, 5*time.Second)
	start := time.Now()
	_, err := c.getTitles(context.Background(), "/x")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("err %v after %v; want a failure soon after the stall", err, time.Since(start))
	}
	if !errors.Is(err, ErrUnreachable) && !errors.Is(err, ErrBadAnswer) {
		t.Fatalf("err %v; want unreachable or bad answer", err)
	}
}
