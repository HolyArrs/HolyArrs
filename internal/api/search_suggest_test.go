package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

type suggestAnswer struct {
	Q           string       `json:"q"`
	Completions []string     `json:"completions"`
	Items       []SuggestRow `json:"items"`
}

func suggestOf(t *testing.T, b []byte) suggestAnswer {
	t.Helper()
	var x suggestAnswer
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x
}

// suggestRig is the ask rig with live search on, five "Radio" movies and a
// server suggest index of five artists, four albums and four songs.
func suggestRig(t *testing.T, delay time.Duration) (*askRig, *atomic.Int32) {
	r := newAskRig(t)
	movies := []metadata.Movie{}
	for i := 1; i <= 5; i++ {
		movies = append(movies, metadata.Movie{Name: fmt.Sprintf("Radio Days %d", i), Year: 2000 + i, IDs: map[string]string{"tmdb": fmt.Sprint(910000 + i)}})
	}
	r.api.s.Metadata = &metadata.Static{Movies: movies}
	r.api.s.LiveSearch = true
	calls := &atomic.Int32{}
	r.api.s.MusicSuggest = func(ctx context.Context, q string) ([]metadata.MusicSuggestion, error) {
		calls.Add(1)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		out := []metadata.MusicSuggestion{}
		for i := 1; i <= 5; i++ {
			out = append(out, metadata.MusicSuggestion{Kind: "artist", Name: fmt.Sprintf("Radiant %d", i), P: 1 - float64(i)/10})
		}
		for i := 1; i <= 4; i++ {
			out = append(out, metadata.MusicSuggestion{Kind: "album", Name: fmt.Sprintf("Radial %d", i), Artist: "Lab Band", P: 0.5},
				metadata.MusicSuggestion{Kind: "song", Name: fmt.Sprintf("Radio Song %d", i), Artist: "Lab Band", P: 0.4})
		}
		return out, nil
	}
	return r, calls
}

// route: GET /api/v1/search/suggest
func TestSearchSuggestMaxThreePerKind(t *testing.T) {
	r, calls := suggestRig(t, 0)
	got := suggestOf(t, r.owner.call("GET", "/api/v1/search/suggest?q=radi", "", 200))
	if len(got.Items) == 0 || len(got.Items) > suggestRows {
		t.Fatalf("items %d: %+v", len(got.Items), got.Items)
	}
	per := map[string]int{}
	for _, it := range got.Items {
		per[it.Kind]++
		if !strings.HasPrefix(strings.ToLower(it.Name), "radi") {
			t.Fatalf("row %+v does not match", it)
		}
	}
	for k, n := range per {
		if n > suggestPerKind {
			t.Fatalf("%d %s rows (max %d): %+v", n, k, suggestPerKind, got.Items)
		}
	}
	if per["movie"] == 0 || per["artist"] == 0 {
		t.Fatalf("both sources must answer: %+v", per)
	}
	if len(got.Completions) == 0 || len(got.Completions) > suggestWords {
		t.Fatalf("completions %v", got.Completions)
	}
	for _, c := range got.Completions {
		if !strings.HasPrefix(c, "radi") {
			t.Fatalf("completion %q", c)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("server suggest calls %d", calls.Load())
	}
	// One letter answers nothing and asks no source.
	if got := suggestOf(t, r.owner.call("GET", "/api/v1/search/suggest?q=r", "", 200)); len(got.Items) != 0 || calls.Load() != 1 {
		t.Fatalf("one letter %+v", got)
	}
	r.owner.call("GET", "/api/v1/search/suggest?q=radi&kind=books", "", 400)
	// Music scope: music rows only.
	for _, it := range suggestOf(t, r.owner.call("GET", "/api/v1/search/suggest?q=radi&kind=music", "", 200)).Items {
		if it.Kind == "movie" {
			t.Fatalf("music scope shows %+v", it)
		}
	}
}

func TestSearchSuggestFlagOff(t *testing.T) {
	r, _ := suggestRig(t, 0)
	r.api.s.LiveSearch = false
	r.owner.call("GET", "/api/v1/search/suggest?q=radi", "", 404)
}

func TestSearchSuggestKidNoMusic(t *testing.T) {
	r, calls := suggestRig(t, 0)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET max_rating = 'PG' WHERE id = ?`, r.ana.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got := suggestOf(t, r.ana.call("GET", "/api/v1/search/suggest?q=radi", "", 200))
	for _, it := range got.Items {
		if it.Kind == "artist" || it.Kind == "album" || it.Kind == "song" || it.TitleID == 0 {
			t.Fatalf("a Kid sees %+v", it)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("a Kid's typing must not read the music index")
	}
}

func TestSearchSuggestSlowSourceLosesItsSlots(t *testing.T) {
	r, _ := suggestRig(t, 3*time.Second)
	start := time.Now()
	got := suggestOf(t, r.owner.call("GET", "/api/v1/search/suggest?q=radi", "", 200))
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("answer took %v", d)
	}
	for _, it := range got.Items {
		if it.Kind == "artist" {
			t.Fatalf("the slow source must lose its slots: %+v", it)
		}
	}
	if len(got.Items) == 0 {
		t.Fatal("the fast source must still answer")
	}
}

func TestSuggestScoreAndPick(t *testing.T) {
	if s := suggestScore(1, 1, "have"); s != 1 {
		t.Fatalf("score %v", s)
	}
	if s := suggestScore(0.5, 0, ""); s != 0.3 {
		t.Fatalf("score %v", s)
	}
	if suggestText("Radiohead", "radi") != 1 || suggestText("The Radio Dept", "radi") != 0.8 || suggestText("Paradise", "radi") != 0.5 {
		t.Fatal("text match")
	}
	rows := []SuggestRow{}
	for i := 0; i < 5; i++ {
		rows = append(rows, SuggestRow{Kind: "song", Name: fmt.Sprint("Radi ", i), score: float64(i)})
	}
	words, out := pickSuggest(rows, "radi")
	if len(out) != 3 || out[0].Name != "Radi 4" || len(words) != 3 || words[0] != "radi 4" {
		t.Fatalf("pick %v %+v", words, out)
	}
}

// A missing album without a cover gets the music picture route, and the
// route knows its names (the same chain as Explore's rows).
func TestSuggestMissingAlbumGetsCoverRoute(t *testing.T) {
	r := newAskRig(t)
	items := []SuggestRow{
		{Kind: "album", Name: "Radial", Sub: "Lab Band", MBID: "abc", State: "missing"},
		{Kind: "album", Name: "Kept", Sub: "Lab Band", MBID: "def", Art: "https://x/y.jpg"},
		{Kind: "album", Name: "No id", Sub: "Lab Band"},
		{Kind: "song", Name: "Song", MBID: "ghi"},
	}
	r.api.coverSuggestAlbums(items)
	if items[0].Art != musicArtPrefix+"album/abc" {
		t.Fatalf("missing album art %q", items[0].Art)
	}
	if items[1].Art != "https://x/y.jpg" || items[2].Art != "" || items[3].Art != "" {
		t.Fatalf("other rows changed: %+v", items[1:])
	}
	r.api.music.mu.Lock()
	n := r.api.music.names["album|abc"]
	r.api.music.mu.Unlock()
	if n.Title != "Radial" || n.Artist != "Lab Band" {
		t.Fatalf("route names %+v", n)
	}
	r.api.music.prefetching.Wait()
}
