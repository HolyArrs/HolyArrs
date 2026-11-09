package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func pendingService(t *testing.T, h http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Service{base: srv.URL, access: func(context.Context) (string, error) { return "tok", nil }, http: srv.Client()}
}

func TestPending202ReaskedThenSucceeds(t *testing.T) {
	var n atomic.Int32
	s := pendingService(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"partial":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"artists":[{"mbid":"5b11f4ce-a62d-471e-81fc-a69a8278c7da","name":"Nirvana"}]}`))
	})
	got, err := s.MusicSimilar(context.Background(), "5b11f4ce-a62d-471e-81fc-a69a8278c7da")
	if err != nil || len(got) != 1 || n.Load() != 2 {
		t.Fatalf("got %v err %v calls %d", got, err, n.Load())
	}
}

func TestPending202CappedIsNotAnError(t *testing.T) {
	var n atomic.Int32
	s := pendingService(t, func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"partial":true}`))
	})
	start := time.Now()
	_, err := s.MusicSimilar(context.Background(), "5b11f4ce-a62d-471e-81fc-a69a8278c7da")
	if !errors.Is(err, ErrServicePending) || errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("err %v", err)
	}
	if n.Load() != pendingTries+1 || time.Since(start) > 9*time.Second {
		t.Fatalf("calls %d took %v", n.Load(), time.Since(start))
	}
}

func TestPendingDetailKeepsPartialData(t *testing.T) {
	s := pendingService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"partial":true,"source":"musicbrainz","source_id":"5b11f4ce-a62d-471e-81fc-a69a8278c7da","completeness":"partial","data":{"source":"musicbrainz","kind":"artist","source_id":"5b11f4ce-a62d-471e-81fc-a69a8278c7da"}}`))
	})
	item, err := s.detail(context.Background(), "musicbrainz", "artist", "5b11f4ce-a62d-471e-81fc-a69a8278c7da")
	if !errors.Is(err, ErrServicePending) || item.Kind != "artist" {
		t.Fatalf("item %+v err %v", item, err)
	}
}
