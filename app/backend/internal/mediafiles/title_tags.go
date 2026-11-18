// Title tags are plain lowercase labels on titles, Downloaders and Sources
// that route a title's grabs and searches.
//
// The rules follow the old apps: a title goes to the Downloaders that share
// one of its tags; when none does, to the Downloaders with no tags at all.
// A Source with tags serves only the titles that share one of them; a
// Source with no tags serves every title.

package mediafiles

import (
	"slices"
	"strings"
	"unicode"
)

// MaxLen and MaxTags bound what one row may hold.
const (
	MaxLen  = 40
	MaxTags = 20
)

// CleanTags trims, lowercases and joins inner spaces with "-", drops empty and
// repeated labels, cuts each to MaxLen and the list to MaxTags, and sorts.
// It never returns nil, so JSON columns always hold a list.
func CleanTags(in []string) []string {
	out := []string{}
	for _, t := range in {
		t = strings.ToLower(strings.Join(strings.FieldsFunc(t, unicode.IsSpace), "-"))
		t = strings.Trim(t, "-,")
		if r := []rune(t); len(r) > MaxLen {
			t = string(r[:MaxLen])
		}
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	if len(out) > MaxTags {
		out = out[:MaxTags]
	}
	return out
}

// Shares says whether two tag lists have a label in common.
func Shares(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// SourceServes says whether a Source with these tags serves a title with
// these tags: always when the Source has none, else when they share one.
func SourceServes(source, title []string) bool {
	return len(source) == 0 || Shares(source, title)
}

// SourceTakesTitle says whether a Source with these tags is read for a title:
// it serves the title's own tags, or shares a tag with the title's Pick rules.
// A title in a folder that asks only tagged Sources (only) is not served by
// an untagged Source. Search and the feed both use it.
func SourceTakesTitle(source, title, rules []string, only bool) bool {
	if only && len(title) > 0 && len(source) == 0 {
		return false
	}
	return SourceServes(source, title) || Shares(source, CleanTags(rules))
}

// SourceTags is the tags a copy of a title has when Sources are chosen for it:
// the title's own plus those of its Library folder when that folder asks only
// tagged Sources (folder). only says the folder does. Each list is cleaned by
// itself, so a title that carries as many tags as it may keeps the folder's.
func SourceTags(title, folder []string) (tags []string, only bool) {
	if len(folder) == 0 {
		return title, false
	}
	tags = CleanTags(title)
	for _, t := range CleanTags(folder) {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return tags, true
}

// OnlyTagged says whether a title that asks only tagged Sources really does:
// it has tags and at least one of the Sources it could use shares one, so it
// is never left with nothing to ask.
func OnlyTagged(only bool, title []string, sourceTags [][]string) bool {
	if !only || len(title) == 0 {
		return false
	}
	for _, s := range sourceTags {
		if Shares(s, title) {
			return true
		}
	}
	return false
}

// RulesTakeSource says whether Pick rules with these tags let a Source with
// these tags be used: rules with no tags take every Source, rules with tags
// only Sources sharing one (untagged Sources are left out).
// Search and the feed both use it.
func RulesTakeSource(source, rules []string) bool {
	rules = CleanTags(rules)
	return len(rules) == 0 || Shares(source, rules)
}

// Downloaders keeps the entries a title's grab may use: those sharing a tag
// with the title when there are any, else those with no tags. tagsOf reads
// an entry's tags. An empty result means no Downloader takes this title.
func Downloaders[T any](all []T, title []string, tagsOf func(T) []string) []T {
	var shared, untagged []T
	for _, d := range all {
		switch tags := tagsOf(d); {
		case len(title) > 0 && Shares(tags, title):
			shared = append(shared, d)
		case len(tags) == 0:
			untagged = append(untagged, d)
		}
	}
	if len(shared) > 0 {
		return shared
	}
	return untagged
}
