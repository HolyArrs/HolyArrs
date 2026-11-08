package api

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Helpers other api tests share with the trailer tests (now in cmd/holyarrs).

// syncBuf is a log output safe for the loops logging next to a test.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func catchLog(t *testing.T) *syncBuf {
	t.Helper()
	b := &syncBuf{}
	old, flags := log.Writer(), log.Flags()
	log.SetOutput(b)
	t.Cleanup(func() { log.SetOutput(old); log.SetFlags(flags) })
	return b
}

func trailerTMDB(t *testing.T) metadata.Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/genre/movie/list", "/3/genre/tv/list":
			_, _ = w.Write([]byte(`{"genres":[]}`))
		case "/3/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"M11","videos":{"results":[{"key":"teaserKey1","site":"YouTube","type":"Teaser","official":true,"iso_639_1":"en"},{"key":"trailerKey1","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"}]}}`))
		case "/3/movie/12":
			_, _ = w.Write([]byte(`{"id":12,"title":"M12"}`))
		case "/3/movie/13", "/3/movie/14", "/3/movie/15":
			// Several trailers: 13 and 14 have three, 15 four.
			id := strings.TrimPrefix(r.URL.Path, "/3/movie/")
			more := ""
			if id == "15" {
				more = `,{"key":"fourthKey01","site":"YouTube","type":"Teaser","official":false,"iso_639_1":"en"}`
			}
			_, _ = w.Write([]byte(`{"id":` + id + `,"title":"M` + id + `","videos":{"results":[{"key":"ageGated001","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"},{"key":"secondKey01","site":"YouTube","type":"Trailer","official":true,"iso_639_1":"en"},{"key":"thirdKey001","site":"YouTube","type":"Teaser","official":true,"iso_639_1":"en"}` + more + `]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &metadata.TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "fake-read-value", nil }, Gap: time.Millisecond}
}
