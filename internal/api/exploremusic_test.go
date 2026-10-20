package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// genreArtists is fakeArtists plus MusicBrainz genres.
type genreArtists struct {
	*fakeArtists
	calls atomic.Int32
}

func (g *genreArtists) ArtistGenres(_ context.Context, mbid string) ([]string, error) {
	g.calls.Add(1)
	switch {
	case strings.HasPrefix(mbid, "a"), strings.HasPrefix(mbid, "5"):
		return []string{"rock"}, nil
	case strings.HasPrefix(mbid, "b"):
		return []string{"rock", "pop"}, nil
	}
	return []string{"jazz"}, nil
}

func (g *genreArtists) ArtistGenresLinks(ctx context.Context, mbid string) ([]string, []metadata.ArtistLink, error) {
	gs, _ := g.ArtistGenres(ctx, mbid)
	var links []metadata.ArtistLink
	if mbid == mid('a', 1) {
		for i := 1; i <= 4; i++ {
			links = append(links, metadata.ArtistLink{MBID: mid('9', i), Name: fmt.Sprintf("Member %d", i), Type: "member of band"})
		}
		links = append(links, metadata.ArtistLink{MBID: mid('b', 2), Name: "Second Band"}) // owned: left out
	}
	return gs, links, nil
}

func mid(prefix byte, n int) string {
	return fmt.Sprintf("%c%07d-0000-4000-8000-%012d", prefix, n, n)
}

type musicRowsAnswer struct {
	Kid    bool       `json:"kid"`
	Cold   bool       `json:"cold"`
	Genres []string   `json:"genres"`
	Hero   *MusicCard `json:"hero"`
	Rows   []MusicRow `json:"rows"`
}

func (m musicRowsAnswer) row(id string) *MusicRow {
	for i := range m.Rows {
		if m.Rows[i].ID == id || m.Rows[i].Type == id {
			return &m.Rows[i]
		}
	}
	return nil
}

func TestExploreMusicRows(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	now := time.Now().UTC()
	day := func(d int) string { return store.FormatTime(now.AddDate(0, 0, d).Truncate(24 * time.Hour)) }
	owned, other := mid('a', 1), mid('b', 2)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := store.FormatTime(now), `{"kind":"system"}`
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		lf := ex(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES (?, ?, ?, ?, ?, 'music', 'Music', 'ok', '[]')`, ts, by, ts, by, t.TempDir())
		var rules int64
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		artist := func(name, mbid string) int64 {
			return ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
				VALUES (?, ?, ?, ?, 'artist', ?, ?, ?, 'musicbrainz', 'by_hand', 'active')`, ts, by, ts, by, name, name, `{"musicbrainz":"`+mbid+`"}`)
		}
		album := func(title int64, name, rg, typ, sec, date string) int64 {
			return ex(`INSERT INTO parts (created_at, created_by, updated_at, updated_by, title_id, kind, name, release_type, release_date, release_date_precision, source_ids, secondary_types, included)
				VALUES (?, ?, ?, ?, ?, 'album', ?, ?, ?, 'day', ?, ?, 1)`, ts, by, ts, by, title, name, typ, date, `{"musicbrainz":"`+rg+`"}`, sec)
		}
		a := artist("Owned Band", owned)
		b := artist("Second Band", other)
		have := album(a, "Arrived Album", mid('c', 1), "album", "[]", day(-400))
		album(a, "Fresh Album", mid('c', 2), "album", "[]", day(-10))
		album(b, "Fresh EP", mid('c', 3), "ep", "[]", day(-20))
		album(b, "Fresh Two", mid('c', 10), "album", "[]", day(-40))
		album(b, "Fresh Live", mid('c', 4), "album", `["live"]`, day(-5))
		album(a, "Coming Album", mid('c', 5), "album", "[]", day(30))
		album(b, "Old One", mid('c', 6), "album", "[]", day(-3000))
		album(b, "Old Two", mid('c', 7), "album", "[]", day(-2000))
		album(a, "Old Three", mid('c', 8), "album", "[]", day(-1000))
		album(b, "Old Four", mid('c', 9), "album", "[]", day(-900))
		ver := ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main) VALUES (?, ?, ?, ?, ?, 'Main', ?, ?, 'Owned Band', 1)`, ts, by, ts, by, a, rules, lf)
		item := ex(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, part_id, kind) VALUES (?, ?, ?, ?, ?, ?, 'track')`, ts, by, ts, by, a, have)
		slot := ex(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`, ts, by, ts, by, item, ver)
		file := ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by) VALUES (?, ?, ?, ?, ?, ?, 'Owned Band/01.flac', ?, 'imported', 'copy')`,
			ts, by, ts, by, "["+itoa(slot)+"]", ver, day(-2))
		ex("UPDATE slots SET file_id = ? WHERE id = ?", file, slot)
		ex(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, ref, source_ids) VALUES (?, ?, ?, ?, 'artist', '', 'Asked Artist', 'approved', 1, ?, ?, ?)`,
			ts, by, ts, by, ts, "mbid:"+mid('d', 1), `{"musicbrainz":"`+mid('d', 1)+`"}`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	trend := func(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
		l := metadata.MusicTrend{Kind: kind, Region: region, Source: "listenbrainz"}
		for i := 1; i <= 6; i++ {
			it := metadata.MusicTrendItem{Rank: i, Listens: int64(1000 * i)}
			if kind == "album" {
				it.MBID, it.Title, it.Artist, it.ArtistMBID = mid('e', i), fmt.Sprintf("Chart Album %d", i), fmt.Sprintf("Chart Artist %d", i), mid('f', i)
			} else {
				it.MBID, it.Name = mid('f', i), fmt.Sprintf("Chart Artist %d", i)
			}
			l.Items = append(l.Items, it)
		}
		return l, nil
	}
	var similarCalls atomic.Int32
	similar := func(_ context.Context, mbid string) ([]metadata.SimilarArtist, error) {
		similarCalls.Add(1)
		var out []metadata.SimilarArtist
		for i := 1; i <= 5; i++ {
			out = append(out, metadata.SimilarArtist{MBID: mid('5', i+10*int(mbid[0]-'a')), Name: fmt.Sprintf("Like %s %d", mbid[:1], i), Score: int64(10 - i)})
		}
		out = append(out, metadata.SimilarArtist{MBID: other, Name: "Second Band"}) // owned: left out
		return out, nil
	}
	genres := &genreArtists{fakeArtists: apiMusic()}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), MusicTrending: trend, MusicSimilar: similar,
		Library: &library.Service{Store: s, Artists: genres}, Version: "test"}).Handler())
	defer srv.Close()
	get := func(q string) musicRowsAnswer {
		t.Helper()
		resp, err := http.Get(srv.URL + "/api/v1/explore/music" + q)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("%s: %v %v", q, err, resp)
		}
		defer resp.Body.Close()
		var out musicRowsAnswer
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first := get("")
	// Genres fill in the background; wait for every queued lookup.
	for i := 0; i < 100 && genres.calls.Load() < 18; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	m := get("")
	if first.Cold || m.Cold || m.Kid {
		t.Fatalf("cold %v %v kid %v", first.Cold, m.Cold, m.Kid)
	}
	if m.Hero == nil || m.Hero.Name != "Fresh Album" || m.Hero.Kind != "album" || m.Hero.Image != "/api/v1/music-trending/art/album/"+mid('c', 2) {
		t.Fatalf("hero %+v", m.Hero)
	}
	if r := m.row("just_arrived"); r != nil {
		t.Fatalf("one album is under the row's minimum: %+v", r)
	}
	nf := m.row("new_from")
	if nf == nil || len(nf.Items) != 3 || nf.Items[0].Name != "Fresh EP" || !nf.Items[2].Upcoming || nf.Items[2].Name != "Coming Album" {
		t.Fatalf("new from (the hero left out, upcoming last) %+v", nf)
	}
	asks := m.row("your_asks")
	if asks == nil || len(asks.Items) != 1 || asks.Items[0].State != "approved" || asks.Items[0].Name != "Asked Artist" {
		t.Fatalf("asks %+v", asks)
	}
	top := m.row("top_albums")
	if top == nil || top.Shape != "square" || len(top.Items) != 6 || top.Items[0].Listens != 1000 || !strings.Contains(top.Reason, "worldwide") {
		t.Fatalf("top albums %+v", top)
	}
	if r := m.row("trending_artists"); r == nil || r.Shape != "round" {
		t.Fatalf("trending %+v", r)
	}
	because := m.row("because_have")
	if because == nil || len(because.Items) != 5 || because.Shape != "round" || !strings.HasPrefix(because.Title, "Because you have ") {
		t.Fatalf("because %+v", because)
	}
	for _, c := range because.Items {
		if c.ArtistMBID == other {
			t.Fatal("an owned artist in Because you have")
		}
	}
	missing := m.row("missing")
	if missing == nil || len(missing.Items) < 4 || missing.Shape != "square" {
		t.Fatalf("missing %+v", missing)
	}
	for _, c := range missing.Items {
		if c.Name == "Arrived Album" || c.Name == "Fresh Live" || c.Name == "Coming Album" {
			t.Fatalf("not missing: %s", c.Name)
		}
	}
	if len(m.Genres) == 0 || m.Genres[0] != "rock" {
		t.Fatalf("genres %v", m.Genres)
	}
	if r := m.row("genre:rock"); r == nil || !strings.Contains(r.Reason, "rock") {
		t.Fatalf("genre row %+v", m.Rows)
	}
	if r := m.row("collabs"); r == nil || len(r.Items) != 4 || r.Shape != "round" || r.Items[0].Note != "With Owned Band" {
		t.Fatalf("collabs %+v", r)
	}
	// The pop chip keeps only cards tagged pop (Second Band's albums).
	pop := get("?genre=pop")
	for _, r := range pop.Rows {
		for _, c := range r.Items {
			if !hasGenre(c.Genres, "pop") {
				t.Fatalf("%s: %s is not pop", r.ID, c.Name)
			}
		}
	}
	if pop.row("top_albums") != nil || pop.Hero != nil {
		t.Fatal("untagged charts kept under a chip")
	}
	if similarCalls.Load() != 2 {
		t.Fatalf("similar lists not cached: %d", similarCalls.Load())
	}
	// A picture for an album on the page is served (no artwork cache here: 404, not a panic).
	if resp, err := http.Get(srv.URL + "/api/v1/music-trending/art/album/" + mid('c', 2)); err != nil || resp.StatusCode != 404 {
		t.Fatalf("art %v", err)
	}
}

func TestExploreMusicColdStart(t *testing.T) {
	_, s := discoverRig(t, newDiscoverTMDB(t).provider())
	trend := func(_ context.Context, kind, region string) (metadata.MusicTrend, error) {
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
	genres := &genreArtists{fakeArtists: apiMusic()}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: newDiscoverTMDB(t).provider(), MusicTrending: trend, Library: &library.Service{Store: s, Artists: genres}, Version: "test"}).Handler())
	defer srv.Close()
	read := func() musicRowsAnswer {
		resp, err := http.Get(srv.URL + "/api/v1/explore/music?region=FR")
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
	m := read()
	if !m.Cold || m.Hero == nil || m.Hero.Rank != 1 {
		t.Fatalf("cold %+v", m)
	}
	for _, r := range m.Rows {
		if !musicColdTypes[r.Type] {
			t.Fatalf("cold start row %s", r.ID)
		}
	}
	if len(m.Rows) < 10 {
		var ids []string
		for _, r := range m.Rows {
			ids = append(ids, r.ID)
		}
		t.Fatalf("cold start shows %d rows: %v", len(m.Rows), ids)
	}
	for _, id := range []string{"popular_genre:rock", "popular_country:FR", "fresh", "rising", "classic", "mood:chill", "decade:1980", "decade:1990", "decade:2000", "decade:2010"} {
		if m.row(id) == nil {
			t.Fatalf("no %s row", id)
		}
	}
}
