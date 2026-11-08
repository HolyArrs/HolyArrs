package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// coldTrend is the cold-start chart fixture (as TestExploreMusicColdStart).
func coldTrend(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
	l := metadata.MusicTrend{Kind: kind, Region: region, Source: "deezer"}
	years := []int{1981, 1984, 1986, 1989, 1991, 1995, 1997, 1996, 2001, 2004, 2006, 2006, 2015, 2016, 2016, 2011}
	for i := 1; i <= 16; i++ {
		switch kind {
		case "album", "fresh", "classic":
			it := metadata.MusicTrendItem{Rank: i, MBID: mid(kind[0], i), Title: "A", Artist: "B", ArtistMBID: mid('a', i)}
			if kind == "classic" {
				it.ReleaseDate, it.Tags = fmt.Sprintf("%d-01-01", years[i-1]), []string{"chill"}
			}
			l.Items = append(l.Items, it)
		default:
			l.Items = append(l.Items, metadata.MusicTrendItem{Rank: i, MBID: mid('a', i), Name: fmt.Sprintf("Artist %d", i)})
		}
	}
	return l, nil
}

// musicShape is an answer's rows as "id: key key ..." lines (hero first).
func musicShape(m musicRowsAnswer) string {
	var b strings.Builder
	if m.Hero != nil {
		b.WriteString("hero: " + m.Hero.Key + "\n")
	}
	for _, r := range m.Rows {
		b.WriteString(r.ID + " [" + r.Shape + "] " + r.Title + ":")
		for _, c := range r.Items {
			b.WriteString(" " + c.Key)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TestExploreMusicFlagOffGolden pins the old row builder: with
// music_rows_v2 off, the cold-start answer matches the copy recorded
// before the flag existed (HOLYARRS_WRITE_GOLDEN=1 rewrites it).
func TestExploreMusicFlagOffGolden(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	genres := &genreArtists{fakeArtists: apiMusic()}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), Library: &library.Service{Store: s, Artists: genres}, Version: "test", Deps: music.Deps{MusicTrending: coldTrend}}).Handler())
	defer srv.Close()
	read := func() musicRowsAnswer {
		resp, err := http.Get(srv.URL + "/api/v1/explore/music?region=FR&hour=10")
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%v %v", err, resp)
		}
		defer resp.Body.Close()
		var out musicRowsAnswer
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	read()
	for i := 0; i < 100 && genres.calls.Load() < 6; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	got := musicShape(read())
	const path = "testdata/explore_music_v1.golden"
	if os.Getenv("HOLYARRS_WRITE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("old Explore output changed:\n--- got\n%s\n--- want\n%s", got, want)
	}
}
