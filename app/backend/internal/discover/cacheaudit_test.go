package discover

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Finding 5: a picture from a stable address is not immutable for a year.
func TestServeArtworkStableRevalidates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	name := "0123456789abcdef0123456789abcdef.jpg"
	p := filepath.Join(dir, name)
	_ = os.WriteFile(p, []byte("jpeg"), 0o644)
	w := httptest.NewRecorder()
	library.ServeArtwork(w, httptest.NewRequest("GET", "/x", nil), name, p)
	if !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("hashed address %q", w.Header().Get("Cache-Control"))
	}
	_ = os.WriteFile(filepath.Join(dir, ".chk-"+name), []byte(`"e1"`), 0o644)
	w = httptest.NewRecorder()
	library.ServeArtwork(w, httptest.NewRequest("GET", "/x", nil), name, p)
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=86400" || w.Header().Get("ETag") == `"0123456789abcdef0123456789abcdef"` {
		t.Fatalf("stable address %q %q", cc, w.Header().Get("ETag"))
	}
}

// Findings 12 and 14: card art 7 days; person and collection lists a day.
func TestShorterListAndCardKeeps(t *testing.T) {
	t.Parallel()
	if CardArtTTL != 7*24*time.Hour {
		t.Fatalf("card art %v", CardArtTTL)
	}
	if ExploreListTTL["person"] != 24*time.Hour || ExploreListTTL["collection"] != 24*time.Hour {
		t.Fatalf("lists %v", ExploreListTTL)
	}
}

// Details with no trailer are asked again within minutes (the server may
// add a season trailer); full details keep the day.
func TestDetailsWithoutVideosShort(t *testing.T) {
	t.Parallel()
	if d := DetailsTTL(Entry{Details: &metadata.Details{}}); d > 15*time.Minute {
		t.Fatalf("no videos kept %v", d)
	}
	if d := DetailsTTL(Entry{Details: &metadata.Details{Videos: []metadata.DetailVideo{{Key: "k"}}}}); d != TTL["details"] {
		t.Fatalf("videos kept %v", d)
	}
}
