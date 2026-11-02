package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

// MusicCharts is the chart side of the music job (the same value as
// Options.Music when it can answer these).
type MusicCharts interface {
	Charts(ctx context.Context, region, kind, genre string, now time.Time) ([]musictrend.ChartItem, bool)
	Genres(region string) ([]musictrend.Genre, bool)
	Suggest(ctx context.Context, q string, limit int) []musictrend.Suggestion
}

// MusicReleases is the release calendar of the new-music job.
type MusicReleases interface {
	Releases(from, to string, artists map[string]bool, now time.Time) []musictrend.ReleaseItem
}

var (
	regionRe = regexp.MustCompile(`^(global|[A-Za-z]{2})$`)
	slugRe   = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	dayRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	mbidRe   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// onlyParams answers false (and a 400) when the query has a parameter not
// in allowed, or one given twice.
func onlyParams(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	for k, v := range r.URL.Query() {
		ok := len(v) == 1
		found := false
		for _, a := range allowed {
			found = found || a == k
		}
		if !ok || !found {
			problem(w, 400, "invalid_query", 0)
			return false
		}
	}
	return true
}

// writeMusicJSON writes v with a strong ETag and the given max-age.
func writeMusicJSON(w http.ResponseWriter, r *http.Request, v any, maxAge int) {
	b, err := json.Marshal(v)
	if err != nil {
		problem(w, 500, "internal", 0)
		return
	}
	sum := sha256.Sum256(b)
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", "private, max-age="+itoa(maxAge))
	h.Set("Vary", "Authorization")
	h.Set("X-Metadata-Cache", "hit")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func (s *Server) musicCharts() (MusicCharts, bool) {
	c, ok := s.opts.Music.(MusicCharts)
	return c, ok && s.opts.Music != nil
}

// serveMusicCharts answers GET /v1/music/charts?region=&kind=albums|songs&genre=
// with P and F per item (6 h cache): the list source's chart for the
// region first, else Apple Music's; a genre's chart is Apple Music's.
func (s *Server) serveMusicCharts(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "region", "kind", "genre") {
		return
	}
	if s.chartsBuilding(w, r) {
		return
	}
	q := r.URL.Query()
	region, kind, genre := q.Get("region"), q.Get("kind"), q.Get("genre")
	if region == "" {
		region = "global"
	}
	if kind == "" {
		kind = "albums"
	}
	if !regionRe.MatchString(region) || (kind != "albums" && kind != "songs") || (genre != "" && !slugRe.MatchString(genre)) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	c, ok := s.musicCharts()
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	items, ok := c.Charts(r.Context(), region, kind, genre, time.Now())
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "region": strings.ToLower(region), "kind": kind, "genre": genre, "rank_days": musictrend.RankDays, "items": items}, 21600)
}

// serveMusicGenres answers GET /v1/music/genres?region= (tiles).
func (s *Server) serveMusicGenres(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "region") {
		return
	}
	if s.chartsBuilding(w, r) {
		return
	}
	region := r.URL.Query().Get("region")
	if region == "" {
		region = "global"
	}
	if !regionRe.MatchString(region) {
		problem(w, 400, "invalid_query", 0)
		return
	}
	c, ok := s.musicCharts()
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	list, ok := c.Genres(region)
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	// Tile art: a rendered grey photo per genre, or none (the flat hue).
	// Missing tiles render in the background for a later answer.
	art, tiles := "flat", s.opts.GenreTiles
	if tiles.On() {
		art = "photo"
	}
	for i := range list {
		if i >= 24 {
			break
		}
		slug, pageArtists := list[i].Slug, list[i].Artists
		if vs := tiles.Versions(slug); len(vs) > 0 {
			for n, v := range vs {
				name := slug
				if n > 0 {
					name += "." + strconv.Itoa(n)
				}
				list[i].Arts = append(list[i].Arts, "/v1/music/genres/tile/"+name+".jpg?v="+v)
			}
			list[i].Art = list[i].Arts[0]
		}
		tiles.Want(slug, func() []TileArtist {
			if slug == "soundtrack" {
				return scoreComposers
			}
			if len(pageArtists) > 0 {
				// The genre page's artists, in the page's order.
				out := make([]TileArtist, 0, len(pageArtists))
				for _, a := range pageArtists {
					out = append(out, TileArtist{MBID: a.ID, Name: a.Name})
				}
				return out
			}
			// The genre's charted artists, best known first: the more
			// charted albums and songs carry the artist, the higher.
			type cand struct {
				a     TileArtist
				count int
			}
			var cands []*cand
			byName := map[string]*cand{}
			for _, kind := range []string{"albums", "songs"} {
				items, _ := c.Charts(context.Background(), region, kind, slug, time.Now())
				for _, it := range items {
					if k := strings.ToLower(it.Artist); it.Artist != "" {
						if cd, ok := byName[k]; ok {
							cd.count++
							continue
						}
						cd := &cand{a: TileArtist{MBID: it.ArtistMBID, Name: it.Artist}, count: 1}
						byName[k] = cd
						cands = append(cands, cd)
					}
				}
			}
			sort.SliceStable(cands, func(x, y int) bool { return cands[x].count > cands[y].count })
			out := make([]TileArtist, 0, len(cands))
			for _, cd := range cands {
				out = append(out, cd.a)
			}
			for _, fb := range genreFallback[slug] {
				if byName[strings.ToLower(fb.Name)] == nil {
					out = append(out, fb)
				}
			}
			return out
		})
	}
	maxAge := 21600
	if art == "photo" {
		// Tiles fill in over a few minutes; look again soon.
		maxAge = 600
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "region": strings.ToLower(region), "hues": musictrend.GenreHues, "art": art, "genres": list}, maxAge)
}

// serveMusicSuggest answers GET /v1/music/suggest?q= from the chart index
// (no MusicBrainz call; the query is not logged or stored).
func (s *Server) serveMusicSuggest(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "q") {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 100 {
		problem(w, 400, "invalid_query", 0)
		return
	}
	c, ok := s.musicCharts()
	if !ok {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "items": c.Suggest(r.Context(), q, 10)}, 60)
}

// serveMusicReleases answers GET /v1/music/releases?from=&to=&artists=a,b
// &types=album,ep,single (up to 90 artist MBIDs, under the 4 KiB URI
// limit): the daily new-music calendar plus, for the named artists, the
// release groups of their stored discographies first out from..to. No
// outside call is made: a discography not stored yet is queued for its
// walk (in the background) and the answer is marked partial (a minute at
// the edge) until it is.
func (s *Server) serveMusicReleases(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r, "from", "to", "artists", "types") {
		return
	}
	q := r.URL.Query()
	now := time.Now().UTC()
	from, to := q.Get("from"), q.Get("to")
	if from == "" {
		from = now.AddDate(0, 0, -30).Format("2006-01-02")
	}
	if to == "" {
		to = now.AddDate(0, 0, 90).Format("2006-01-02")
	}
	if !dayRe.MatchString(from) || !dayRe.MatchString(to) || from > to {
		problem(w, 400, "invalid_query", 0)
		return
	}
	artists := map[string]bool{}
	var order []string
	if raw := q.Get("artists"); raw != "" {
		for _, a := range strings.Split(strings.ToLower(raw), ",") {
			if !mbidRe.MatchString(a) || len(artists) >= 90 {
				problem(w, 400, "invalid_query", 0)
				return
			}
			if !artists[a] {
				order = append(order, a)
			}
			artists[a] = true
		}
	}
	types := map[string]bool{}
	if raw := q.Get("types"); raw != "" {
		for _, t := range strings.Split(strings.ToLower(raw), ",") {
			if t != "album" && t != "ep" && t != "single" {
				problem(w, 400, "invalid_query", 0)
				return
			}
			types[t] = true
		}
	}
	rel, ok := s.opts.NewMusic.(MusicReleases)
	if (!ok || s.opts.NewMusic == nil) && s.opts.Discog == nil {
		problem(w, 404, "list_unavailable", 0)
		return
	}
	items := []musictrend.ReleaseItem{}
	seen := map[string]bool{}
	if ok && s.opts.NewMusic != nil {
		for _, it := range rel.Releases(from, to, artists, now) {
			if len(types) > 0 && !types[strings.ToLower(it.Type)] {
				continue
			}
			seen[it.MBID] = true
			items = append(items, it)
		}
	}
	partial := false
	if s.opts.Discog != nil {
		for _, id := range order {
			a, stored := s.opts.Discog.Artist(r.Context(), id)
			if !stored {
				s.wantDiscog(context.Background(), discog.ArtistKey(id), fetchq.P3Refresh, false)
				partial = true
				continue
			}
			var mb wikidata.MBArtist
			if b, ok := s.StoredData(r.Context(), "artist", id); ok {
				mb, _ = wikidata.ParseMBArtist(b)
			}
			for _, g := range a.Groups {
				day, kind := g.FirstReleaseDate, strings.ToLower(g.PrimaryType)
				if len(day) != 10 || day < from || day > to || seen[g.ID] || (len(types) > 0 && !types[kind]) || kind == "" {
					continue
				}
				seen[g.ID] = true
				it := musictrend.ReleaseItem{NewItem: musictrend.NewItem{MBID: g.ID, ArtistMBID: id, Title: g.Title, Artist: mb.Name, ReleaseDate: day,
					Type: kind, Sources: []string{"musicbrainz"}}, F14: musictrend.Freshness(day, 14, now)}
				for _, st := range g.SecondaryTypes {
					it.SecondaryTypes = append(it.SecondaryTypes, strings.ToLower(st))
				}
				for _, gn := range mb.Genres {
					if len(it.Tags) < 5 {
						it.Tags = append(it.Tags, strings.ToLower(gn))
					}
				}
				items = append(items, it)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ReleaseDate != items[j].ReleaseDate {
			return items[i].ReleaseDate > items[j].ReleaseDate
		}
		return items[i].MBID < items[j].MBID
	})
	if partial {
		w.Header().Set(partialHeader, "1")
	}
	writeMusicJSON(w, r, map[string]any{"schema_version": 1, "from": from, "to": to, "partial": partial, "items": items}, 21600)
}

// scoreComposers is the Soundtrack genre's fixed shortlist (MusicBrainz
// artist ids): its charted credits are compilations and casts, which have
// no photo, so the tile shows well-known film-score composers instead.
var scoreComposers = []TileArtist{
	{MBID: "e6de1f3b-6484-491c-88dd-6d619f142abc", Name: "Hans Zimmer"},
	{MBID: "53b106e7-0cc6-42cc-ac95-ed8d30a3a98e", Name: "John Williams"},
	{MBID: "78471616-3cb4-4139-a148-aeac3c7d0f79", Name: "Ludwig Göransson"},
	{MBID: "8a399a51-6b93-448e-82c0-f86c83602605", Name: "Hildur Guðnadóttir"},
	{MBID: "a16e47f5-aa54-47fe-87e4-bb8af91a9fdd", Name: "Ennio Morricone"},
}

// genreFallback lists well-known artists for genres whose region chart
// carries too few artists to fill a slideshow; they follow the charted
// artists (MusicBrainz artist ids).
var genreFallback = map[string][]TileArtist{
	"dance": {
		{MBID: "8dd98bdc-80ec-4e93-8509-2f46bafc09a7", Name: "Calvin Harris"}, {MBID: "302bd7b9-d012-4360-897a-93b00c855680", Name: "David Guetta"},
		{MBID: "ae002c5d-aac6-490b-a39a-30aa9e2edf2b", Name: "Skrillex"}, {MBID: "c85cfd6b-b1e9-4a50-bd55-eb725f04f7d5", Name: "Avicii"}, {MBID: "056e4f3e-d505-4dad-8ec1-d04f521cbb56", Name: "Daft Punk"},
	},
	"metal": {
		{MBID: "65f4f0c5-ef9e-490c-aee3-909e7ae6b2ab", Name: "Metallica"}, {MBID: "ca891d65-d9b0-4258-89f7-e6ba29d83767", Name: "Iron Maiden"},
		{MBID: "5182c1d9-c7d2-4dad-afa0-ccfeada921a8", Name: "Black Sabbath"}, {MBID: "a466c2a2-6517-42fb-a160-1087c3bafd9f", Name: "Slipknot"},
	},
	"rock": {
		{MBID: "67f66c07-6e61-4026-ade5-7e782fad3a5d", Name: "Foo Fighters"}, {MBID: "66c662b6-6e2f-4930-8610-912e24c63ed1", Name: "AC/DC"},
		{MBID: "5b11f4ce-a62d-471e-81fc-a69a8278c7da", Name: "Nirvana"}, {MBID: "f59c5520-5f46-4d2c-b2c4-822eabf53419", Name: "Linkin Park"},
	},
	"k-pop": {
		{MBID: "48646387-1664-4c9a-9139-9bfd091b823c", Name: "BLACKPINK"}, {MBID: "142b343d-bf5a-428c-a64f-6d1a7566bbe9", Name: "Stray Kids"},
		{MBID: "49204a7a-ed85-407a-828f-6fd46f1d8126", Name: "NewJeans"}, {MBID: "8da127cc-c432-418f-b356-ef36210d82ac", Name: "TWICE"}, {MBID: "e04d239e-9fa8-49b3-b9b7-9e439c3cb1d1", Name: "SEVENTEEN"},
	},
	"electronic": {
		{MBID: "4a00ec9d-c635-463a-8cd4-eb61725f0c60", Name: "deadmau5"}, {MBID: "5700dcd4-c139-4f31-aa3e-6382b9af9032", Name: "Kraftwerk"},
		{MBID: "1946a82a-f927-40c2-8235-38d64f50d043", Name: "The Chemical Brothers"}, {MBID: "f22942a1-6f70-4f48-866e-238cb2308fbd", Name: "Aphex Twin"},
	},
	"children-s-music": {
		{MBID: "d5d5ba92-4bd4-4975-9806-99339eed3046", Name: "Raffi"}, {MBID: "e684ed8b-bced-4e0f-bdfa-462861891409", Name: "The Wiggles"}, {MBID: "ca91d4fa-d532-426d-933a-14f26d83ab1b", Name: "Laurie Berkner"}, {MBID: "d68700b6-aae7-478a-b75b-5ea8c6851cd4", Name: "Sesame Street"},
	},
}
