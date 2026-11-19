package shows

import (
	"context"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var reTVRegion = regexp.MustCompile(`(?i)\s+\(?(US|UK|AU|CA|NZ|DE|FR|JP|KR)\)?$`)
var reTVAbbreviation = regexp.MustCompile(`^[A-Z]{2,6}$`)

// A scan page caches repeated season queries and permits at most 40 fallback
// requests. A normal match makes no additional metadata requests.
type tvMatcher struct {
	provider  metadata.ShowProvider
	remaining int
	cache     map[string][]metadata.Show
}

func newTVMatcher(p metadata.ShowProvider) *tvMatcher {
	return &tvMatcher{p, 40, map[string][]metadata.Show{}}
}

// A season-only folder can borrow a name from consistent episode filenames.
// Inspect at most 64 already-listed videos, with no extra filesystem walk.
func tvEpisodeName(videos []library.AdoptVideo) string {
	name, count, checked := "", 0, 0
	for _, v := range videos[:min(len(videos), 64)] {
		if tvExtraPath(v.Path) {
			continue
		}
		checked++
		p := releasename.Parse(path.Base(v.Path), releasename.Hints{Show: true})
		if !p.IsShow() || len(p.Show.Episodes)+len(p.Show.Absolute) == 0 || p.Title.Value == "" {
			continue
		}
		candidate := p.Title.Value
		if name != "" && releasename.CleanName(candidate) != releasename.CleanName(name) {
			return ""
		}
		name = candidate
		count++
	}
	if count < 2 || count*5 < checked*4 {
		return ""
	}
	return name
}

func (m *tvMatcher) matchFolder(ctx context.Context, folder string, videos []library.AdoptVideo) (string, int, []library.AdoptMatch, error) {
	name, year, matches, err := m.match(ctx, folder)
	if err != nil || !reTVSeason.MatchString(folder) || len(matches) > 0 && matches[0].Score >= library.MatchUnsure {
		return name, year, matches, err
	}
	fromFiles := tvEpisodeName(videos)
	if fromFiles == "" {
		return name, year, matches, nil
	}
	if _, err := m.search(ctx, fromFiles, true); err != nil {
		return name, year, matches, err
	}
	if _, ok := m.cache[strings.ToLower(fromFiles)]; !ok {
		return name, year, matches, nil
	} // fallback budget exhausted
	n, y, found, err := m.match(ctx, fromFiles)
	if err != nil {
		return name, year, matches, err
	}
	if len(found) == 0 || found[0].Score < library.MatchUnsure {
		return name, year, matches, nil
	}
	for i := range found {
		found[i].Score = min(found[i].Score, 0.85)
	}
	return n, y, found, nil
}
func (m *tvMatcher) search(ctx context.Context, query string, fallback bool) ([]metadata.Show, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := strings.ToLower(query)
	if strings.TrimSpace(key) == "" {
		return nil, nil
	}
	if got, ok := m.cache[key]; ok {
		return got, nil
	}
	if fallback {
		if m.remaining == 0 {
			return nil, nil
		}
		m.remaining--
	}
	got, err := m.provider.SearchShows(ctx, query)
	if err == nil {
		m.cache[key] = got
	}
	return got, err
}
func tvNameScore(name string, year int, sh metadata.Show) float64 {
	best := 0.0
	for _, candidate := range append([]string{sh.Name}, sh.OtherNames...) {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		best = max(best, library.NameScore(name, year, candidate, sh.Year))
		if tvAcronymMatch(name, candidate) {
			best = max(best, min(0.85, library.NameScore(candidate, year, candidate, sh.Year)))
		}
		// A regional qualifier can yield a suggestion, never an automatic match.
		if base := reTVRegion.ReplaceAllString(name, ""); base != name {
			best = max(best, min(0.75, library.NameScore(base, year, candidate, sh.Year)))
		}
	}
	return best
}
func tvWords(s string) []string {
	return strings.FieldsFunc(strings.ReplaceAll(s, "&", " and "), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Expand initials only against words actually returned by the provider.
// Require a shared literal word so an acronym alone cannot identify a show.
func tvAcronymMatch(name, candidate string) bool {
	a, b := tvWords(name), tvWords(candidate)
	if len(a) > 32 || len(b) > 32 {
		return false
	}
	shared := false
	for _, x := range a {
		for _, y := range b {
			if len(x) > 2 && strings.EqualFold(x, y) {
				shared = true
			}
		}
	}
	if !shared {
		return false
	}
	seen := map[[2]int]bool{}
	var match func(int, int) bool
	match = func(i, j int) bool {
		if i == len(a) {
			return j == len(b)
		}
		if j == len(b) || seen[[2]int{i, j}] {
			return false
		}
		seen[[2]int{i, j}] = true
		if strings.EqualFold(a[i], b[j]) && match(i+1, j+1) {
			return true
		}
		if reTVAbbreviation.MatchString(a[i]) {
			initials := ""
			for k := j; k < len(b) && k < j+6; k++ {
				r := []rune(b[k])
				initials += strings.ToUpper(string(r[0]))
				if k > j && initials == a[i] && match(i+1, k+1) {
					return true
				}
			}
		}
		return false
	}
	return match(0, 0)
}
func tvFallbackQueries(name string) []string {
	words := strings.Fields(name)
	if len(words) < 2 {
		return nil
	}
	var out []string
	if reTVAbbreviation.MatchString(words[0]) {
		out = append(out, strings.Trim(strings.Join(words[1:], " "), " -"))
	}
	if reTVAbbreviation.MatchString(words[len(words)-1]) {
		out = append(out, strings.Trim(strings.Join(words[:len(words)-1], " "), " -"))
	}
	return out
}
