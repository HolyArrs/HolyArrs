package library

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"golang.org/x/text/unicode/norm"
)

// matchCandidate keeps normal searches and prefetch unchanged. Only uncertain
// collection-pack or year-first release matches may use one detail request.
// An uncertain pack may also use one extra search for a director suffix.
// No title or person names are special-cased.
func matchCandidate(ctx context.Context, md metadata.Provider, rel string) (string, int, []AdoptMatch, error) {
	budget := 2
	return matchCandidateWithBudget(ctx, md, rel, &budget)
}

// Bound additional metadata work across a whole scan, not just per movie.
const scanEvidenceRequests = 40

func takeEvidenceRequest(budget *int) bool {
	if *budget <= 0 {
		return false
	}
	*budget--
	return true
}

func matchCandidateWithBudget(ctx context.Context, md metadata.Provider, rel string, budget *int) (string, int, []AdoptMatch, error) {
	name, year, matches, err := matchFolder(ctx, md, matchingLookup(rel))
	// A release without a year can contain a franchise prefix. Offer a
	// bounded suffix suggestion only after the full query found nothing;
	// never promote this heuristic to an automatic match.
	if err == nil && year == 0 && len(matches) == 0 && !reTMDBTag.MatchString(rel) && hasTechnicalParentheses(path.Base(rel)) {
		words := strings.Fields(name)
		if len(words) >= 5 && takeEvidenceRequest(budget) {
			query := strings.Join(words[1:], " ")
			movies, e := md.Search(ctx, query, 0)
			if e != nil {
				return name, year, matches, e
			}
			for _, m := range movies {
				if m.IDs["tmdb"] != "" && sameMovieName(query, m.Name) {
					matches = append(matches, AdoptMatch{TMDB: m.IDs["tmdb"], Name: m.Name, Year: m.Year, Score: 0.8, Poster: m.Poster})
				}
			}
			if len(matches) > 4 {
				matches = matches[:4]
			}
		}
	}
	pack := collectionPackPath(rel)
	yearFirst := yearFirstRelease.MatchString(path.Base(rel))
	if err != nil || year == 0 || (!pack && !yearFirst) || reTMDBTag.MatchString(rel) {
		return name, year, matches, err
	}
	for i := range matches {
		m := &matches[i]
		if m.Year == year && sequelName(name) == sequelName(m.Name) {
			m.Score = 1
		}
	}
	sortMatches(matches)
	if len(matches) > 0 && matches[0].Score >= MatchSure {
		return name, year, matches, nil
	}

	// A redundant pack sequel number may differ from the canonical title.
	// Keep it reviewable: matching the same year is evidence, not proof that
	// the number is extraneous. Never discard numbers in normal movie titles.
	words := strings.Fields(name)
	if pack && len(words) > 1 {
		if n, e := strconv.Atoi(words[len(words)-1]); e == nil && n > 0 && n < 100 {
			base := strings.Join(words[:len(words)-1], " ")
			for i := range matches {
				if matches[i].Year == year && sameMovieName(base, matches[i].Name) {
					matches[i].Score = max(matches[i].Score, 0.89)
				}
			}
			sortMatches(matches)
		}
	}

	// A unique exact-year result can be verified against its alternate names
	// or director credits. Multiple results stay ambiguous.
	var possible []AdoptMatch
	for _, m := range matches {
		titlePrefix := strings.HasPrefix(releasename.CleanName(name), releasename.CleanName(m.Name)+" ")
		if m.Year == year && (m.Score >= MatchUnsure || yearFirst || titlePrefix) {
			possible = append(possible, m)
		}
	}
	if len(matches) == 0 && pack && len(words) >= 3 {
		// Two-word person suffixes are common. Do not try every word split:
		// bound the network work, and require credits before accepting it.
		query := strings.Join(words[:len(words)-2], " ")
		if !takeEvidenceRequest(budget) {
			return name, year, matches, nil
		}
		movies, e := md.Search(ctx, query, year)
		if e != nil {
			if ctx.Err() != nil {
				return name, year, matches, ctx.Err()
			}
			return name, year, matches, e
		}
		for _, m := range movies {
			if m.Year == year && m.IDs["tmdb"] != "" && sameMovieName(query, m.Name) {
				possible = append(possible, AdoptMatch{TMDB: m.IDs["tmdb"], Name: m.Name, Year: m.Year, Poster: m.Poster})
			}
		}
	}
	if len(possible) != 1 {
		return name, year, matches, nil
	}
	m := possible[0]
	if !takeEvidenceRequest(budget) {
		return name, year, matches, nil
	}
	detail, e := md.Movie(ctx, "tmdb:"+m.TMDB)
	if e != nil {
		if ctx.Err() != nil {
			return name, year, matches, ctx.Err()
		}
		return name, year, matches, e
	}
	if detail.IDs["tmdb"] != m.TMDB || detail.Year != year || !sameMovieName(detail.Name, m.Name) {
		return name, year, matches, nil
	}
	verified := false
	for _, alias := range detail.OtherNames {
		if alias != "" && sameMovieName(name, alias) {
			verified = true
		}
	}
	for _, credits := range [][]string{detail.Directors, detail.People} {
		for _, person := range credits {
			if person != "" && sameMovieName(name, detail.Name+" "+person) {
				verified = true
			}
		}
	}
	if verified {
		m.Score = 1
		for i := range matches {
			if matches[i].TMDB == m.TMDB {
				matches[i] = m
				sortMatches(matches)
				return name, year, matches, nil
			}
		}
		matches = append(matches, m)
		sortMatches(matches)
		if len(matches) > 4 {
			matches = matches[:4]
		}
	}
	return name, year, matches, nil
}

func sameMovieName(a, b string) bool { return releasename.CleanName(a) == releasename.CleanName(b) }

// Normalize numeral spellings only in multiword titles: a standalone X is
// still X, while a sequel's III and superscript digits can compare equally.
var joinedSequelNumber = regexp.MustCompile(`([[:alpha:]])([0-9]+)$`)

func sequelName(name string) string {
	name = joinedSequelNumber.ReplaceAllString(norm.NFKC.String(name), "$1 $2")
	words := strings.Fields(releasename.CleanName(name))
	if len(words) > 1 {
		for i := 1; i < len(words); i++ {
			switch words[i] {
			case "ii":
				words[i] = "2"
			case "iii":
				words[i] = "3"
			case "iv":
				words[i] = "4"
			case "v":
				words[i] = "5"
			case "vi":
				words[i] = "6"
			case "vii":
				words[i] = "7"
			case "viii":
				words[i] = "8"
			case "ix":
				words[i] = "9"
			case "x":
				words[i] = "10"
			}
		}
	}
	return strings.Join(words, " ")
}

func sortMatches(ms []AdoptMatch) {
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Score > ms[j].Score })
}
