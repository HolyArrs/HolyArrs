package subtitles

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Sleeping disks: a subtitle look or search does not read the video again
// when its size, modified time and built-in settings match what was stored
// last time. Beside files are still listed every time.

// sameSizeSameTime replaces the video's bytes with body of the same length
// and restores its modified time, so only the content differs.
func sameSizeSameTime(t *testing.T, p string, body []byte) {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) != st.Size() {
		t.Fatalf("body is %d bytes, file is %d", len(body), st.Size())
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func builtInRows(t *testing.T, f *fix) []Subtitle {
	t.Helper()
	rows, err := ListFor(context.Background(), f.st.Read(), f.movieFile)
	if err != nil {
		t.Fatal(err)
	}
	var out []Subtitle
	for _, r := range rows {
		if r.Place == "built_in" {
			out = append(out, r)
		}
	}
	return out
}

func TestVideoCache_LookSkipsUnchangedVideo(t *testing.T) {
	f := newFix(t)
	b, err := os.ReadFile("container/testdata/lab-built-in.mkv")
	if err != nil {
		t.Fatal(err)
	}
	p := f.abs(movieRel)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: f.st}
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	first := builtInRows(t, f)
	if len(first) == 0 {
		t.Fatal("no built-in rows from the lab file")
	}

	// Same size and time with different bytes keeps the stored tracks, which
	// proves the video was not read again and the rows were not rewritten.
	sameSizeSameTime(t, p, bytes.Repeat([]byte{0}, len(b)))
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	second := builtInRows(t, f)
	if len(second) != len(first) {
		t.Fatalf("unchanged video was read again: %d built-in rows, had %d", len(second), len(first))
	}
	for i := range first {
		if second[i].ID != first[i].ID || !second[i].CreatedAt.Equal(first[i].CreatedAt) {
			t.Errorf("built-in row %d was deleted and inserted again: %+v then %+v", i, first[i], second[i])
		}
	}

	// A new modified time reads the video again, which is now blank with no tracks.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := builtInRows(t, f); len(got) != 0 {
		t.Errorf("changed video was not read again: %+v", got)
	}
}

// Turning built-in subtitles off or on reads the video again even when the
// video did not change.
func TestVideoCache_LookReadsAgainWhenBuiltInSettingsChange(t *testing.T) {
	f := newFix(t)
	b, err := os.ReadFile("container/testdata/lab-built-in.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.abs(movieRel), b, 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: f.st}
	ctx := context.Background()
	wishAll(t, f, Want{Language: "en"})
	set := ReadSettings(ctx, f.st.Read())
	set.UseBuiltIn = false
	_ = SaveSettings(ctx, f.st, set, domain.System)
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := builtInRows(t, f); len(got) != 0 {
		t.Fatalf("built-in off still listed %+v", got)
	}
	set.UseBuiltIn = true
	_ = SaveSettings(ctx, f.st, set, domain.System)
	if _, err := s.Look(ctx, f.movieFile, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := builtInRows(t, f); len(got) == 0 {
		t.Fatal("turning built-in on did not read the video")
	}
}

// hashSeen wraps the fake shaped like OpenSubtitles and records the moviehash
// of each search.
type hashSeen struct {
	mu     sync.Mutex
	next   http.Handler
	hashes []string
}

func (h *hashSeen) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/subtitles") {
		h.mu.Lock()
		h.hashes = append(h.hashes, r.URL.Query().Get("moviehash"))
		h.mu.Unlock()
	}
	h.next.ServeHTTP(w, r)
}

func TestVideoCache_SearchUsesStoredHashForUnchangedVideo(t *testing.T) {
	f := newFix(t)
	ctx := context.Background()
	p := f.abs(movieRel)
	if err := os.WriteFile(p, bytes.Repeat([]byte{1}, 200<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	wishAll(t, f, Want{Language: "pt"}) // the site has nothing, so every search asks again
	seen := &hashSeen{next: osLab()}
	srv := httptest.NewServer(seen)
	t.Cleanup(srv.Close)
	v, err := secrets.Open(ctx, f.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: f.st, Vault: v}
	s.Enqueue = func(context.Context, string, *domain.Subject, map[string]any, string) error { return nil }
	if _, err := AddSite(ctx, f.st, v, SiteInput{Kind: "opensubtitles", URL: srv.URL + "/api/v1", AllowLocalNetwork: true,
		APIKey: mkKey, Username: mkUser, Password: mkPass, Limits: &SiteLimits{GapMS: 1, PerWindow: 1000, WindowSeconds: 3600}}, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	sameSizeSameTime(t, p, bytes.Repeat([]byte{2}, 200<<10))
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	seen.mu.Lock()
	hashes := append([]string(nil), seen.hashes...)
	seen.mu.Unlock()
	if len(hashes) != 2 || hashes[0] == "" {
		t.Fatalf("searches sent hashes %q; want two with a hash", hashes)
	}
	if hashes[1] != hashes[0] {
		t.Errorf("unchanged video was hashed again: %s then %s", hashes[0], hashes[1])
	}

	// A new modified time hashes the video again.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(ctx, f.movieFile, SearchOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	seen.mu.Lock()
	defer seen.mu.Unlock()
	if len(seen.hashes) != 3 || seen.hashes[2] == seen.hashes[0] {
		t.Errorf("changed video kept its old hash: %q", seen.hashes)
	}
}
