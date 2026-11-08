package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ErrBadEpisode is an asked-for episode number below 1.
var ErrBadEpisode = errors.New("episode numbers start at 1")

// wholeShow says a show ask asks for every season.
func wholeShow(a domain.Ask) bool {
	return !a.FutureOnly && len(a.Seasons) == 0 && len(a.Episodes) == 0
}

// pickEpisodes checks single asked-for episodes against the show's seasons
// (when known) and tidies them: one entry per season, numbers sorted once,
// seasons asked for whole left out, oldest season first.
func pickEpisodes(asked []domain.AskEpisodes, whole []int, known []metadata.Season) ([]domain.AskEpisodes, error) {
	by := map[int][]int{}
	for _, e := range asked {
		if known != nil && !slices.ContainsFunc(known, func(k metadata.Season) bool { return k.Number == e.Season }) {
			return nil, ErrNoSuchSeason
		}
		for _, n := range e.Episodes {
			if n < 1 {
				return nil, ErrBadEpisode
			}
			if !slices.Contains(by[e.Season], n) {
				by[e.Season] = append(by[e.Season], n)
			}
		}
	}
	out := []domain.AskEpisodes{}
	for season, eps := range by {
		if len(eps) == 0 || slices.Contains(whole, season) {
			continue
		}
		slices.Sort(eps)
		out = append(out, domain.AskEpisodes{Season: season, Episodes: eps})
	}
	slices.SortFunc(out, func(x, y domain.AskEpisodes) int { return x.Season - y.Season })
	return out, nil
}

// episodesOf is the asked-for episodes of one season (nil: none singly).
func episodesOf(eps []domain.AskEpisodes, season int) []int {
	for _, e := range eps {
		if e.Season == season {
			return e.Episodes
		}
	}
	return nil
}

// coversParts says whether an ask o already asks for the whole seasons want
// and the single episodes wantEps.
func coversParts(o domain.Ask, want []int, wantEps []domain.AskEpisodes) bool {
	if wholeShow(o) {
		return true
	}
	if len(want) == 0 && len(wantEps) == 0 {
		return false
	}
	for _, n := range want {
		if !slices.Contains(o.Seasons, n) {
			return false
		}
	}
	for _, e := range wantEps {
		if slices.Contains(o.Seasons, e.Season) {
			continue
		}
		have := episodesOf(o.Episodes, e.Season)
		for _, n := range e.Episodes {
			if !slices.Contains(have, n) {
				return false
			}
		}
	}
	return true
}

// widenParts joins what an ask asks for with more seasons and episodes;
// either being the whole show gives the whole show.
func widenParts(a *domain.Ask, want []int, wantEps []domain.AskEpisodes) {
	if wholeShow(*a) || (len(want) == 0 && len(wantEps) == 0) {
		a.Seasons, a.Episodes = []int{}, []domain.AskEpisodes{}
		return
	}
	a.Seasons = widen(a.Seasons, want)
	a.Episodes, _ = pickEpisodes(append(slices.Clone(a.Episodes), wantEps...), a.Seasons, nil)
}

// episodesNotKept lists which single episodes a show in the Library does
// not keep up to date yet (the show, its season and the episode must all
// be kept). An episode the Library does not know yet is asked for.
func episodesNotKept(ctx context.Context, db querier, titleID int64, eps []domain.AskEpisodes) ([]domain.AskEpisodes, error) {
	var showKeep bool
	if err := db.QueryRowContext(ctx, episodesNotKeptSQL, titleID).Scan(&showKeep); err != nil {
		return nil, err
	}
	out := []domain.AskEpisodes{}
	for _, e := range eps {
		var missing []int
		for _, n := range e.Episodes {
			var keep bool
			err := db.QueryRowContext(ctx, episodesNotKeptSQL2,
				titleID, e.Season, n).Scan(&keep)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if !keep || !showKeep {
				missing = append(missing, n)
			}
		}
		if len(missing) > 0 {
			out = append(out, domain.AskEpisodes{Season: e.Season, Episodes: missing})
		}
	}
	return out, nil
}

// episodeWords says which single episodes an ask is for: "episode 3 of
// season 2" or "5 episodes of season 2".
func episodeWords(eps []domain.AskEpisodes) []string {
	var out []string
	for _, e := range eps {
		if len(e.Episodes) == 1 {
			out = append(out, fmt.Sprintf("episode %d of season %d", e.Episodes[0], e.Season))
		} else {
			out = append(out, fmt.Sprintf("%d episodes of season %d", len(e.Episodes), e.Season))
		}
	}
	return out
}

// partsWords says what a show ask with single episodes is for: "season 1
// and 5 episodes of season 2".
func partsWords(seasons []int, eps []domain.AskEpisodes) string {
	parts := episodeWords(eps)
	if len(seasons) > 0 {
		parts = append([]string{seasonWords(seasons)}, parts...)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
