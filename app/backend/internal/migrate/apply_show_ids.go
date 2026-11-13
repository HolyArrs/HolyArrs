package migrate

import (
	"errors"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var errShowNotFound = errors.New("the show could not be found by its ids or its name")

// withShowIDs runs add with the show's own id; only when the metadata
// source does not know that show it tries the IMDb id, then the folder's
// name and year. A different TMDB id from the old app is never used to
// replace an id that found the show.
func (r *run) withShowIDs(t *TitlePlan, add func(id string) error) error {
	err := add(t.ID)
	if t.Kind != "show" || !isNoSuchShow(err) {
		return err
	}
	for _, id := range r.otherShowIDs(t) {
		if err = add(id); !isNoSuchShow(err) {
			return err
		}
	}
	return errShowNotFound
}

// isNoSuchShow tells a show the source does not know from any other
// failure; an adopted folder's error arrives as text.
func isNoSuchShow(err error) bool {
	return err != nil && (errors.Is(err, metadata.ErrShowNotFound) || strings.Contains(err.Error(), metadata.ErrShowNotFound.Error()))
}

var folderYear = regexp.MustCompile(`\s*[\(\[]\d{4}[\)\]]|\s*[\{\[][^}\]]*[\}\]]`)

func plainName(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(folderYear.ReplaceAllString(s, "")) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c > 127 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// otherShowIDs are the ids to try after t.ID found nothing.
func (r *run) otherShowIDs(t *TitlePlan) []string {
	var out []string
	seen := map[string]bool{t.ID: true}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if v := t.IDs["imdb"]; v != "" {
		add("imdb:" + v)
	}
	if r.m.Library == nil || r.m.Library.Shows == nil {
		return out
	}
	// By name last: only a result with the same name (and year, when both
	// know one) counts.
	for _, src := range []string{"tmdb", "tvmaze", "tvdb"} {
		p, err := r.m.Library.Shows.Get(src)
		if err != nil {
			continue
		}
		list, err := p.SearchShows(r.ctx, strings.TrimSpace(folderYear.ReplaceAllString(t.Folder, "")))
		if err != nil {
			continue
		}
		want, found := plainName(t.Folder), false
		for _, sh := range list {
			if plainName(sh.Name) != want || (t.Year != 0 && sh.Year != 0 && sh.Year != t.Year) {
				continue
			}
			for _, k := range []string{"tvdb", "tmdb", "imdb"} {
				if v := sh.IDs[k]; v != "" {
					add(k + ":" + v)
					found = true
				}
			}
		}
		if found {
			break
		}
	}
	return out
}
