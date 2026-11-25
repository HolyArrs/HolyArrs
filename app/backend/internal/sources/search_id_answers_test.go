package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// frenchReleases are what a French Torznab site answers for an id search of
// a movie it names in French.
var frenchReleases = []string{
	"Dune.Deuxieme.Partie.2024.MULTi.1080p.WEB.H264-FRATERNiTY",
	"Dune.Deuxième.Partie.2024.TRUEFRENCH.720p.WEB-DL.x264-GRP",
	"Dune.Deuxieme.Partie.2024.FRENCH.2160p.WEB.H265-SUPPLY",
	"Dune.Deuxieme.Partie.2024.VFF.1080p.BluRay.x264-QTZ",
}

// idSite is a fake Torznab site with movie search by q, imdbid and tmdbid.
// byID answers an id search; attrs are the extra attrs each item carries;
// byWords answers a search by words.
type idSite struct {
	mu      sync.Mutex
	asks    []url.Values
	byID    []string
	attrs   string
	byWords []string
}

func (s *idSite) handler(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	if v.Get("t") == "caps" {
		_, _ = w.Write([]byte(`<caps><searching><search available="yes" supportedParams="q"/><movie-search available="yes" supportedParams="q,imdbid,tmdbid"/></searching><categories><category id="2000" name="Movies"/></categories></caps>`))
		return
	}
	s.mu.Lock()
	s.asks = append(s.asks, v)
	s.mu.Unlock()
	names, attrs := s.byWords, ""
	if v.Get("imdbid") != "" || v.Get("tmdbid") != "" {
		names, attrs = s.byID, s.attrs
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
	for i, n := range names {
		fmt.Fprintf(&b, `<item><title>%s</title><pubDate>%s</pubDate><enclosure url="http://src/%d.torrent" length="%d" type="application/x-bittorrent"/><torznab:attr name="seeders" value="40"/>%s</item>`,
			n, time.Now().UTC().Add(-time.Hour).Format(time.RFC1123Z), i, int64(4000)<<20, attrs)
	}
	b.WriteString(`</channel></rss>`)
	_, _ = w.Write([]byte(b.String()))
}

func (s *idSite) words() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.asks {
		if a.Get("q") != "" {
			n++
		}
	}
	return n
}

func duneQuery() Query {
	return Query{Title: "Dune: Part Two", Year: 2024, IMDb: "tt15239678", TMDb: 693134}
}

// The id answer decides; the search by words is a fallback, and only an
// answer that is plainly for other titles is dropped.
func TestSearch_IDAnswers(t *testing.T) {
	t.Parallel()
	ours := `<torznab:attr name="imdbid" value="tt15239678"/><torznab:attr name="tmdbid" value="693134"/>`
	other := `<torznab:attr name="tmdbid" value="1"/>`
	for _, tc := range []struct {
		name       string
		site       *idSite
		want       int
		wantWords  int
		wantDrops  int
		wantByName bool
	}{
		{name: "509 replay: French names carrying the movie's ids", site: &idSite{byID: frenchReleases, attrs: ours}, want: 4},
		{name: "only the TMDB id on each release", site: &idSite{byID: frenchReleases, attrs: `<torznab:attr name="tmdbid" value="693134"/>`}, want: 4},
		{name: "no ids on the releases, nothing found by words", site: &idSite{byID: frenchReleases}, want: 4, wantWords: 1},
		{name: "no ids on the releases, words find the title", site: &idSite{byID: frenchReleases, byWords: []string{"Dune.Part.Two.2024.1080p.WEB-DL-GRP"}}, want: 1, wantWords: 1, wantDrops: 4, wantByName: true},
		{name: "no ids on the releases, loose words find other titles", site: &idSite{byID: frenchReleases, byWords: []string{"Dunes.of.Sand.2023.1080p.WEB-DL-GRP", "Part.Two.Live.2019.720p-X"}}, want: 6, wantWords: 1, wantByName: true},
		{name: "site ignored the id: every release another title's", site: &idSite{byID: notitleLatest, attrs: other}, want: 0, wantWords: 1, wantDrops: len(notitleLatest)},
		{name: "site ignored the id, words find the title", site: &idSite{byID: notitleLatest, attrs: other, byWords: []string{"Dune.Part.Two.2024.1080p.WEB-DL-GRP"}}, want: 1, wantWords: 1, wantDrops: len(notitleLatest), wantByName: true},
		{name: "English names, no ids", site: &idSite{byID: []string{"Dune.Part.Two.2024.1080p.WEB-DL-GRP"}}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := tc.site
			srv := serve(t, site.handler)
			q := duneQuery()
			q.drops = &dropTally{}
			items, byID, err := New(Config{URL: srv.URL + "/api", Protocol: "torrent", AllowLocal: true}).search(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != tc.want || site.words() != tc.wantWords || q.drops.take() != tc.wantDrops || byID == tc.wantByName {
				t.Fatalf("got %d items %v, %d word searches, byID %v; asks %v", len(items), titles(items), site.words(), byID, site.asks)
			}
		})
	}
}

// The ids on each result are read, TMDB too.
func TestParseItems_ReadsIDs(t *testing.T) {
	items, err := parseItems([]byte(`<rss xmlns:torznab="x"><channel><item><title>A</title><link>http://a</link>
<torznab:attr name="tmdbid" value="693134"/><torznab:attr name="imdbid" value="tt0015239"/><torznab:attr name="tvdbid" value="7"/></item></channel></rss>`), "torrent")
	if err != nil || len(items) != 1 || items[0].TMDb != 693134 || items[0].IMDb != "15239" || items[0].TVDB != 7 {
		t.Fatalf("%+v %v", items, err)
	}
}

// End to end: a French site answers the id search with French names; the
// releases are stored as the movie's (matched by the Source's id), not lost.
func TestSearchSlot_FrenchIDAnswerKept(t *testing.T) {
	site := &idSite{byID: frenchReleases, attrs: `<torznab:attr name="imdbid" value="tt15239678"/>`}
	srv := serve(t, site.handler)
	f := seed(t, srv.URL+"/api")
	// The id is trusted only for a movie that is out.
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET digital_date = ?`, database.FormatTime(time.Now().AddDate(-1, 0, 0)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true, "choose": false})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["releases"].(float64); int(n) != 4 {
		t.Fatalf("result %v", j.Result)
	}
	var matched int
	if err := f.s.Read().QueryRow(`SELECT count(*) FROM releases WHERE job_id = ? AND json_array_length(matched_slot_ids) > 0`, j.ID).Scan(&matched); err != nil {
		t.Fatal(err)
	}
	if matched != 4 {
		t.Fatalf("want all 4 matched to the movie by its id, got %d", matched)
	}
}

// A site that ignored the id and sent only other titles: the job says how
// many were left out and from where, and the releases route says so.
func TestSearchSlot_DroppedCountedAndShown(t *testing.T) {
	site := &idSite{byID: notitleLatest, attrs: `<torznab:attr name="tmdbid" value="1"/>`}
	srv := serve(t, site.handler)
	f := seed(t, srv.URL+"/api")
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true, "choose": false})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	if n, _ := j.Result["dropped"].(float64); int(n) != len(notitleLatest) {
		t.Fatalf("result %v", j.Result)
	}
	ls := lastSearch(context.Background(), f.s.Read(), titleOf(t, f), nil)
	if ls == nil || ls.Found != 0 || ls.Dropped != len(notitleLatest) || len(ls.DroppedBy) != 1 || ls.At == nil {
		t.Fatalf("last search %+v", ls)
	}
}

// The three empty states: never searched (nil), searched and found nothing,
// and the last search failed.
func TestLastSearch_States(t *testing.T) {
	site := &idSite{}
	srv := serve(t, site.handler)
	f := seed(t, srv.URL+"/api")
	if ls := lastSearch(context.Background(), f.s.Read(), titleOf(t, f), nil); ls != nil {
		t.Fatalf("never searched: %+v", ls)
	}
	j := runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slotID}, map[string]any{"by_hand": true, "choose": false})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	ls := lastSearch(context.Background(), f.s.Read(), titleOf(t, f), nil)
	if ls == nil || ls.Failed || ls.Found != 0 || ls.Dropped != 0 {
		t.Fatalf("found nothing: %+v", ls)
	}
	exec(t, f, `UPDATE jobs SET state = 'failed'`)
	if ls := lastSearch(context.Background(), f.s.Read(), titleOf(t, f), nil); ls == nil || !ls.Failed {
		t.Fatalf("failed: %+v", ls)
	}
}

func titleOf(t *testing.T, f fixture) int64 {
	var id int64
	if err := f.s.Read().QueryRow("SELECT id FROM titles LIMIT 1").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
