package discover

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// textDetails answers details and counts them; a title's texts are kept
// once its details were read (or when listed in kept).
type textDetails struct {
	*metadata.Static
	mu    sync.Mutex
	asked []string
	kept  map[string]bool
}

func (f *textDetails) Discover(context.Context, string, string, string, string) ([]metadata.DiscoverItem, error) {
	return nil, nil
}

func (f *textDetails) Details(_ context.Context, kind, id string) (metadata.Details, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, kind+":"+id)
	f.kept[kind+":"+id] = true
	return metadata.Details{DiscoverItem: metadata.DiscoverItem{Kind: kind, TMDBID: id, Name: "Title " + id}}, nil
}

func (f *textDetails) KeptText(_ context.Context, kind, id, _ string) (metadata.TitleText, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return metadata.TitleText{}, f.kept[kind+":"+id]
}

func (f *textDetails) askedFor() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

// Details kept before titles carried their texts are read again by the
// warmer, so Explore cards get their names in each person's language; a
// fresh page with its texts is left alone.
func TestTitleWarmerReadsDetailsWithoutTexts(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &textDetails{Static: metadata.Lab(), kept: map[string]bool{"movie:6": true}}
	c := &Cache{}
	fresh := Entry{At: time.Now(), Details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "abcdefgh"}}}}
	c.Put("details|movie|5", fresh)
	c.Put("details|movie|6", fresh)
	// The trailer key is each title's last step (trailers play by default):
	// two calls, both titles done and the store left alone.
	var warmed atomic.Int32
	w := &TitleWarmer{Store: func() *database.Store { return s }, Metadata: func() metadata.Provider { return f }, Cache: func() *Cache { return c },
		WarmExtras: func(context.Context, string, string) {}, TrailerKey: func(context.Context, string, string, string) { warmed.Add(1) }}
	w.Warm("en", [][2]string{{"movie", "5"}, {"movie", "6"}})
	deadline := time.Now().Add(20 * time.Second)
	for warmed.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := f.askedFor(); len(got) != 1 || got[0] != "movie:5" {
		t.Fatalf("details asked for %v, want only movie:5", got)
	}
}
