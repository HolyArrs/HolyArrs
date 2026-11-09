package metadata

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestServiceTopReadsAllServices(t *testing.T) {
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/lists/service-top" || r.Header.Get("Authorization") == "" {
			t.Error("bad request " + r.URL.String())
		}
		if r.URL.RawQuery != "region=US" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"schema_version":1,"region":"US","attribution":"Streaming data by Streaming Availability (Movie of the Night)","services":[{"service":"netflix","name":"Netflix","fetched_at":"2026-10-20T14:05:12Z","source":"streaming-availability","items":[{"rank":1,"tmdb_id":285322,"kind":"tv","title":"Below","poster_path":"/b.jpg"}]}]}`))
	})
	l, err := s.ServiceTop(context.Background(), "US")
	if err != nil || l.Attribution == "" || len(l.Services) != 1 || l.Services[0].Source != "streaming-availability" || l.Services[0].Items[0].TMDBID != 285322 || l.Services[0].Items[0].Kind != "tv" {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := s.ServiceTop(context.Background(), "GB"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing region: %v", err)
	}
}
