//go:build livesubs

package subtitles

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// TestLiveSubs runs one real search on each site that needs no account.
// It is meant to be run by hand only.
func TestLiveSubs(t *testing.T) {
	cases := []struct {
		kind string
		q    sites.Query
		v    Video
	}{
		{"podnapisi", sites.Query{Kind: "movie", Title: "The Matrix", Year: 1999, Languages: []string{"en"}},
			Video{Kind: "movie", Title: "The Matrix", Year: 1999, ReleaseName: "The.Matrix.1999.1080p.BluRay.x264-GRP"}},
		{"gestdown", sites.Query{Kind: "episode", Title: "Breaking Bad", Season: 1, Episode: 1, Languages: []string{"en"}},
			Video{Kind: "episode", Title: "Breaking Bad", Season: 1, Episode: 1, ReleaseName: "Breaking.Bad.S01E01.720p.BluRay.x264-GRP"}},
	}
	for _, tc := range cases {
		if only := os.Getenv("LIVESUBS_ONLY"); only != "" && only != tc.kind {
			continue
		}
		k, _ := sites.KindOf(tc.kind)
		s, err := sites.New(tc.kind, sites.Config{Name: k.Name, URL: liveURL(tc.kind, k.DefaultURL)})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cs, err := s.Search(ctx, tc.q)
		cancel()
		var se *sites.Error
		if errors.As(err, &se) {
			t.Logf("%s: site error kind=%s retry=%v msg=%s", tc.kind, se.Kind, se.RetryAfter, se.Msg)
		} else if err != nil {
			t.Logf("%s: error: %v", tc.kind, err)
		}
		t.Logf("%s: %d candidates", tc.kind, len(cs))
		for i, c := range cs {
			if i >= 3 {
				break
			}
			sc := ScoreCandidate(tc.v, Want{Language: "en"}, c)
			t.Logf("  %q lang=%s releases=%d score=%d refused=%q", c.Title, c.Language, len(c.ReleaseNames), sc.Total, sc.Refused)
		}
	}
}

// liveURL lets an environment variable such as LIVESUBS_PODNAPISI_URL replace
// the default address for a single run.
func liveURL(kind, def string) string {
	if kind == "podnapisi" {
		if u := os.Getenv("LIVESUBS_PODNAPISI_URL"); u != "" {
			return u
		}
	}
	return def
}
