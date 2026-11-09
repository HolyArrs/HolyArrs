package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func showcaseDir(t *testing.T, marked bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "holyarrs-showcase-18728")
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "holyarrs.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if marked {
		if err := os.WriteFile(filepath.Join(dir, showcaseMarker), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestShowcaseAcceptsMarkedShowcase(t *testing.T) {
	if err := checkShowcase(showcaseOptions{Runtime: showcaseDir(t, true), AppURL: "http://127.0.0.1:18728"}); err != nil {
		t.Fatal(err)
	}
}

func TestShowcaseRefusesShippedPorts(t *testing.T) {
	dir := showcaseDir(t, true)
	for _, u := range []string{"http://127.0.0.1:18730", "http://192.168.1.10:18731"} {
		if err := checkShowcase(showcaseOptions{Runtime: dir, AppURL: u}); err == nil || !strings.Contains(err.Error(), "shipped") {
			t.Fatalf("%s: got %v", u, err)
		}
	}
}

func TestShowcaseRefusesOtherFolders(t *testing.T) {
	// No marker.
	if err := checkShowcase(showcaseOptions{Runtime: showcaseDir(t, false), AppURL: "http://127.0.0.1:18728"}); err == nil {
		t.Fatal("unmarked folder accepted")
	}
	// Not named as the showcase (a production config folder).
	prod := filepath.Join(t.TempDir(), "holyarrs-18730")
	_ = os.MkdirAll(filepath.Join(prod, "config"), 0o755)
	_ = os.WriteFile(filepath.Join(prod, showcaseMarker), []byte("x"), 0o644)
	if err := checkShowcase(showcaseOptions{Runtime: prod, AppURL: "http://127.0.0.1:18728"}); err == nil {
		t.Fatal("non-showcase folder accepted")
	}
	// The lab, even when named and marked like the showcase.
	lab := showcaseDir(t, true)
	_ = os.WriteFile(filepath.Join(lab, "config", labMarker), []byte("x"), 0o644)
	if err := checkShowcase(showcaseOptions{Runtime: lab, AppURL: "http://127.0.0.1:18728"}); err == nil || !strings.Contains(err.Error(), "lab") {
		t.Fatalf("lab accepted: %v", err)
	}
	// No database.
	empty := filepath.Join(t.TempDir(), "holyarrs-showcase-x")
	_ = os.MkdirAll(empty, 0o755)
	_ = os.WriteFile(filepath.Join(empty, showcaseMarker), []byte("x"), 0o644)
	if err := checkShowcase(showcaseOptions{Runtime: empty, AppURL: "http://127.0.0.1:18728"}); err == nil {
		t.Fatal("folder without a database accepted")
	}
}

// TestShowcaseCatalog checks the catalog covers each scenario with real ids
// and lists no title twice.
func TestShowcaseCatalog(t *testing.T) {
	seen := map[string]bool{}
	plans := map[string]int{}
	asks := map[string]int{}
	for _, m := range showcaseMovieList {
		if m.TMDB == "" || m.Name == "" || m.Scenario == "" {
			t.Fatalf("incomplete entry %+v", m)
		}
		if seen["movie:"+m.TMDB] {
			t.Fatalf("movie %s listed twice", m.TMDB)
		}
		seen["movie:"+m.TMDB] = true
		if m.Quality != "" && qualityTags[m.Quality] == "" {
			t.Fatalf("%s: unknown quality %q", m.Name, m.Quality)
		}
		plans[m.Plan]++
		if m.Ask != "" {
			asks[m.Ask]++
			if m.AskedBy == "" {
				t.Fatalf("%s: ask without an asker", m.Name)
			}
		}
	}
	for _, p := range []string{planDownloading, planQueued, planPaused, planStalled, planFailed, planGone, planGrab, planUpgrade} {
		if plans[p] == 0 {
			t.Errorf("no movie shows plan %q", p)
		}
	}
	for _, a := range []string{"asked", "approved", "turned_down"} {
		if asks[a] == 0 {
			t.Errorf("no movie ask is %q", a)
		}
	}
	for _, s := range showcaseShowList {
		if seen["show:"+s.TMDB] {
			t.Fatalf("show %s listed twice", s.TMDB)
		}
		seen["show:"+s.TMDB] = true
		if qualityTags[s.Quality] == "" {
			t.Fatalf("%s: unknown quality %q", s.Name, s.Quality)
		}
	}
	for _, d := range showcaseEpisodeDownloads {
		if !seen["show:"+d.TMDB] {
			t.Fatalf("episode download for a show not in the catalog: %s", d.TMDB)
		}
	}
	var complete, downloading, asked bool
	for _, a := range showcaseArtistList {
		if len(a.MBID) != 36 {
			t.Fatalf("%s: not a MusicBrainz id", a.Name)
		}
		complete = complete || a.Complete
		downloading = downloading || a.Downloading
		asked = asked || a.Ask
	}
	if !complete || !downloading || !asked {
		t.Fatal("music scenarios missing")
	}
	kid := false
	for _, m := range showcaseMembers {
		kid = kid || m.MaxRating != ""
	}
	if !kid || len(showcaseMembers) < 2 {
		t.Fatal("a member and a Kid member are needed")
	}
}
