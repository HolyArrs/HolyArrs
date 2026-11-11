package metadata

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// AnimeEpisodes remaps episodes of one AniDB season onto a TVDB or TMDB
// season: Start..End shift by Offset; Pairs remap single episodes.
type AnimeEpisodes struct {
	AniDBSeason int         `json:"anidb_season"`
	TVDBSeason  *int        `json:"tvdb_season,omitempty"`
	TMDBSeason  *int        `json:"tmdb_season,omitempty"`
	Start       int         `json:"start,omitempty"`
	End         int         `json:"end,omitempty"`
	Offset      int         `json:"offset,omitempty"`
	Pairs       []AnimePair `json:"pairs,omitempty"`
}

// AnimePair maps one AniDB episode to target episodes (0: none).
type AnimePair struct {
	AniDB int   `json:"anidb"`
	To    []int `json:"to"`
}

// AnimeEntry is one anime (an AniDB title, usually one season or cour)
// with its IDs and how its episodes sit in the TMDB/TVDB show.
type AnimeEntry struct {
	AniDB      int64           `json:"anidb,omitempty"`
	AniList    int64           `json:"anilist,omitempty"`
	MAL        int64           `json:"mal,omitempty"`
	Kitsu      int64           `json:"kitsu,omitempty"`
	Type       string          `json:"type,omitempty"`
	TVDB       int64           `json:"tvdb,omitempty"`
	TVDBSeason *int            `json:"tvdb_season,omitempty"`
	TVDBOffset int             `json:"tvdb_offset,omitempty"`
	TMDBTV     int64           `json:"tmdb_tv,omitempty"`
	TMDBSeason *int            `json:"tmdb_season,omitempty"`
	TMDBOffset int             `json:"tmdb_offset,omitempty"`
	TMDBMovies []int64         `json:"tmdb_movies,omitempty"`
	Absolute   bool            `json:"absolute,omitempty"`
	Episodes   []AnimeEpisodes `json:"episodes,omitempty"`
}

// AnimeTitle is one AniList title's variants.
type AnimeTitle struct {
	AniList int64  `json:"anilist"`
	Romaji  string `json:"romaji,omitempty"`
	English string `json:"english,omitempty"`
	Native  string `json:"native,omitempty"`
}

// AnimeMap is the metadata server's anime ID mapping for one title.
type AnimeMap struct {
	Entries   []AnimeEntry `json:"entries"`
	Titles    []AnimeTitle `json:"titles"`
	UpdatedAt string       `json:"updated_at"`
}

// AnimeMap reads the server's mapping for an ID: by is tmdb_tv,
// tmdb_movie, tvdb, anilist or mal. ErrNotFound when no list knows it.
func (s *Service) AnimeMap(ctx context.Context, by, id string) (AnimeMap, error) {
	var out AnimeMap
	q := url.Values{}
	switch by {
	case "tmdb_tv":
		q.Set("tmdb", id)
	case "tmdb_movie":
		q.Set("tmdb", id)
		q.Set("kind", "movie")
	case "tvdb", "anilist", "mal":
		q.Set(by, id)
	default:
		return out, ErrServiceInvalid
	}
	if !serviceID("tmdb", id) {
		return out, ErrServiceInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	tok, err := s.access(ctx)
	if err != nil || tok == "" || strings.ContainsAny(tok, "\r\n") {
		return out, ErrServiceAccess
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/anime/map?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return out, ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return out, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return out, ErrServiceBusy
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return out, ErrServiceAccess
	case resp.StatusCode != http.StatusOK:
		return out, ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil {
		return AnimeMap{}, ErrServiceInvalid
	}
	return out, nil
}

// LooksAnime is the rule without a mapping: TMDB genre Animation and a
// Japanese origin (origin country JP, or Japanese as the original language
// when TMDB gives no country).
func LooksAnime(genres, originCountry []string, originalLanguage string) bool {
	if !slices.ContainsFunc(genres, func(g string) bool { return strings.EqualFold(g, "Animation") }) {
		return false
	}
	if len(originCountry) > 0 {
		return slices.Contains(originCountry, "JP")
	}
	return originalLanguage == "ja"
}

// AltNames are the AniList title variants (romaji, English, native) not
// equal to name or to each other, case folded, in that order.
func (m AnimeMap) AltNames(name string) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(name)): true}
	out := []string{}
	for _, t := range m.Titles {
		for _, v := range []string{t.Romaji, t.English, t.Native} {
			k := strings.ToLower(strings.TrimSpace(v))
			if k != "" && !seen[k] {
				seen[k] = true
				out = append(out, strings.TrimSpace(v))
			}
		}
	}
	return out
}

// maxAnimeSeason bounds the TMDB season numbers taken from the server.
const maxAnimeSeason = 1000

// AnimeSeason is how one TMDB season of an anime maps: the AniList title
// of that cour, and the absolute number of its first episode.
type AnimeSeason struct {
	Number       int         `json:"number"`
	AniList      int64       `json:"anilist,omitempty"`
	MAL          int64       `json:"mal,omitempty"`
	Title        *AnimeTitle `json:"title,omitempty"`
	Offset       int         `json:"offset,omitempty"`
	AbsoluteFrom int         `json:"absolute_from,omitempty"`
	// Parts are the AniDB titles (cours) inside this season, in order,
	// each with the TMDB episode it starts after.
	Parts []AnimeSeasonPart `json:"parts,omitempty"`
}

// AnimeSeasonPart is one AniDB title inside a TMDB season.
type AnimeSeasonPart struct {
	AniList int64       `json:"anilist,omitempty"`
	Offset  int         `json:"offset"`
	Title   *AnimeTitle `json:"title,omitempty"`
}

// Seasons maps the TMDB seasons (number to episode count) of a show onto
// the anime entries: per season its cours with their episode offsets, and
// the absolute number of the season's first episode (specials, season 0,
// have none). Seasons with no entry are left out.
func (m AnimeMap) Seasons(counts map[int]int) []AnimeSeason {
	titles := map[int64]*AnimeTitle{}
	for i := range m.Titles {
		titles[m.Titles[i].AniList] = &m.Titles[i]
	}
	by := map[int]*AnimeSeason{}
	for _, e := range m.Entries {
		if e.TMDBSeason == nil || e.Absolute || *e.TMDBSeason < 0 || *e.TMDBSeason > maxAnimeSeason {
			continue
		}
		n := *e.TMDBSeason
		s := by[n]
		if s == nil {
			s = &AnimeSeason{Number: n}
			by[n] = s
		}
		s.Parts = append(s.Parts, AnimeSeasonPart{AniList: e.AniList, Offset: e.TMDBOffset, Title: titles[e.AniList]})
	}
	var nums []int
	for n := range by {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	out := []AnimeSeason{}
	// Absolute numbers count the episodes of every earlier TMDB season
	// (specials left out); only seasons that exist are walked.
	var known []int
	for n := range counts {
		if n >= 1 && n <= maxAnimeSeason {
			known = append(known, n)
		}
	}
	slices.Sort(known)
	for _, n := range nums {
		if n < 1 {
			continue
		}
		abs := 1
		for _, k := range known {
			if k >= n {
				break
			}
			abs += max(0, counts[k])
		}
		by[n].AbsoluteFrom = abs
	}
	for _, n := range nums {
		s := by[n]
		slices.SortStableFunc(s.Parts, func(a, b AnimeSeasonPart) int { return a.Offset - b.Offset })
		first := s.Parts[0]
		s.AniList, s.Offset, s.Title = first.AniList, first.Offset, first.Title
		for _, e := range m.Entries {
			if e.AniList == first.AniList {
				s.MAL = e.MAL
				break
			}
		}
		if len(s.Parts) == 1 {
			s.Parts = nil
		}
		out = append(out, *s)
	}
	return out
}

// SetReleaseNumbers gives the episodes of TMDB show tmdbID the TVDB-style
// season and episode that release names use, when a TMDB season holds
// several parts: in Mashle's TMDB season 1, episode 17 is S02E05. Only
// entries of this show with a regular TMDB and TVDB season count; a map
// with one part per season changes nothing. seasons is TMDB's own season
// list. It reports whether the map knows the show and is current, even
// when it changes nothing.
func (m AnimeMap) SetReleaseNumbers(tmdbID string, seasons []Season, eps []Episode) bool {
	have := map[int]bool{}
	for _, s := range seasons {
		have[s.Number] = true
	}
	for _, ep := range eps {
		have[ep.Season] = true
	}
	by := map[int][]AnimeEntry{}
	for _, e := range m.Entries {
		if strconv.FormatInt(e.TMDBTV, 10) == tmdbID && e.TMDBSeason != nil && *e.TMDBSeason > 0 && !have[*e.TMDBSeason] {
			// The map names a season TMDB no longer has: it is out of date.
			return false
		}
		if strconv.FormatInt(e.TMDBTV, 10) != tmdbID || e.Absolute || e.TMDBSeason == nil || e.TVDBSeason == nil || *e.TMDBSeason < 1 || *e.TVDBSeason < 1 {
			continue
		}
		by[*e.TMDBSeason] = append(by[*e.TMDBSeason], e)
	}
	if !slices.ContainsFunc(slices.Collect(maps.Values(by)), func(es []AnimeEntry) bool { return len(es) > 1 }) {
		return len(by) > 0
	}
	for i := range eps {
		ep := &eps[i]
		if ep.Season < 1 {
			continue
		}
		var part *AnimeEntry
		for j, e := range by[ep.Season] {
			if e.TMDBOffset < ep.Number && (part == nil || e.TMDBOffset > part.TMDBOffset) {
				part = &by[ep.Season][j]
			}
		}
		if part == nil {
			continue
		}
		season, number := *part.TVDBSeason, ep.Number-part.TMDBOffset+part.TVDBOffset
		if season != ep.Season || number != ep.Number {
			ep.AltSeason, ep.AltEpisode = season, number
		}
	}
	return true
}

// setSeasonSizes numbers TMDB's regular episodes the way release names do
// when sizes, the episode counts of release seasons 1, 2, ..., split TMDB's
// seasons in order: each TMDB season but the last holds whole release
// seasons, and the last reaches into each of the rest and covers its dated
// episodes. Episodes past the end stay in the last release season, as with
// the anime map. Layouts that agree or do not fit change nothing.
func setSeasonSizes(eps []Episode, sizes []int) {
	count, dated, seasons := map[int]int{}, map[int]int{}, 0
	for _, e := range eps {
		if e.Season > 0 {
			count[e.Season]++
			if e.AirDay != "" {
				dated[e.Season]++
			}
			seasons = max(seasons, e.Season)
		}
	}
	if seasons == 0 || len(sizes) <= seasons || slices.ContainsFunc(sizes, func(n int) bool { return n < 1 }) {
		return
	}
	first, j := map[int]int{}, 0
	for s := 1; s < seasons; s++ {
		first[s] = j
		sum := 0
		for sum < count[s] && j < len(sizes) {
			sum += sizes[j]
			j++
		}
		if sum != count[s] {
			return
		}
	}
	first[seasons] = j
	rest, total := sizes[j:], 0
	for _, n := range rest {
		total += n
	}
	if len(rest) == 0 || total < dated[seasons] || total-rest[len(rest)-1] >= count[seasons] {
		return
	}
	for i := range eps {
		ep := &eps[i]
		if ep.Season < 1 {
			continue
		}
		k, n := first[ep.Season], ep.Number
		for k < len(sizes)-1 && n > sizes[k] {
			n -= sizes[k]
			k++
		}
		if k+1 != ep.Season || n != ep.Number {
			ep.AltSeason, ep.AltEpisode = k+1, n
		}
	}
}
