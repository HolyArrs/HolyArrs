package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func artworkEvents(t *testing.T, s *database.Store) int {
	t.Helper()
	_, n, err := s.Events(context.Background(), database.EventFilter{Kinds: []string{domain.EventArtworkChanged}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// New cover art on a title that had some is a moment; the first poster of a
// new title, or the same poster again, is not.
func TestCoverArtChangeIsAnEvent(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	pics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff\xe0 " + r.URL.Path))
	}))
	t.Cleanup(pics.Close)
	svc := &Service{Store: s, Now: func() time.Time { return now },
		Artwork: &ArtworkCache{Dir: filepath.Join(t.TempDir(), "art"), Allowed: []string{pics.URL + "/"}, HTTP: &http.Client{}}}
	svc.CachePoster(ctx, id, pics.URL+"/a.jpg", domain.System)
	if n := artworkEvents(t, s); n != 0 {
		t.Fatalf("first poster made %d events", n)
	}
	svc.CachePoster(ctx, id, pics.URL+"/a.jpg", domain.System)
	if n := artworkEvents(t, s); n != 0 {
		t.Fatalf("same poster made %d events", n)
	}
	svc.CachePoster(ctx, id, pics.URL+"/b.jpg", domain.System)
	if n := artworkEvents(t, s); n != 1 {
		t.Fatalf("new poster made %d events, want 1", n)
	}
}
