package musictrend

import (
	"context"
	"regexp"
	"strings"
	"time"
)

// The last pass over a served chart (audit 2026-10-29): an item is
// explicit when any source says so; Apple's " - EP" or " - Single" title
// tag becomes the release type; a credit of several artists is linked by
// its first artist; an album still without an id is found in the slim
// copy by its artist and title; an album or song appears once; and a row
// dated after today is a pre-order.

// releaseTags are Apple's title tags and the release type each names.
var releaseTags = []struct{ tag, typ string }{{" - EP", "ep"}, {" - Single", "single"}}

// SplitReleaseTag answers a title without Apple's " - EP" or " - Single"
// tag and the release type the tag named ("" when there is none).
func SplitReleaseTag(title string) (string, string) {
	for _, t := range releaseTags {
		if base, ok := strings.CutSuffix(title, t.tag); ok && strings.TrimSpace(base) != "" {
			return strings.TrimSpace(base), t.typ
		}
	}
	return title, ""
}

// creditSeps split a credit of several artists, first artist first.
var creditSeps = []string{" feat. ", " ft. ", " featuring ", " with ", " & ", ", ", " x ", " X ", " and "}

// FirstArtist answers the first artist of a credit ("Drake & 21 Savage":
// "Drake"); a single name comes back as it is.
func FirstArtist(credit string) string {
	cut := len(credit)
	for _, sep := range creditSeps {
		if i := strings.Index(credit, sep); i > 0 && i < cut {
			cut = i
		}
	}
	return strings.TrimSpace(credit[:cut])
}

// SetSlimAlbum sets the slim copy's album finder: an artist's release
// group whose title matches (no call).
// SetExactArtist sets how a chart row's own artist id (or its album's
// barcode) is resolved to a MusicBrainz artist exactly: source and id are
// the chart source's ("applemusic"), codes the row's barcodes; "" when it
// does not resolve. Call before Run.
func (s *Service) SetExactArtist(f func(ctx context.Context, source, id, name string, codes []string) string) {
	s.opts.ExactArtist = f
}

func (s *Service) SetSlimAlbum(f func(ctx context.Context, artistMBID, title string) string) {
	s.opts.SlimAlbum = f
}

func fixKey(title, artist string) string {
	t, _ := SplitReleaseTag(title)
	return norm(baseTitle(t)) + "|" + norm(FirstArtist(artist))
}

// explicitIndex answers the albums or songs any kept Apple chart marks
// explicit.
func (s *Service) explicitIndex(kind string) map[string]bool {
	s.amu.RLock()
	defer s.amu.RUnlock()
	out := map[string]bool{}
	for _, c := range s.apple {
		list := c.albums
		if kind == "songs" {
			list = c.songs
		}
		for _, e := range list {
			if e.Explicit {
				out[fixKey(e.Name, e.Artist)] = true
			}
		}
	}
	return out
}

// finish is the last pass over a served chart (see the top of the file).
func (s *Service) finish(ctx context.Context, kind string, items []ChartItem, now time.Time) []ChartItem {
	n := len(items)
	explicit := s.explicitIndex(kind)
	today := now.UTC().Format("2006-01-02")
	seen := map[string]bool{}
	out := items[:0]
	for _, it := range items {
		if t, typ := SplitReleaseTag(it.Title); typ != "" {
			it.Title, it.ReleaseType = t, typ
		}
		k := fixKey(it.Title, it.Artist)
		it.Explicit = it.Explicit || explicit[k]
		if it.ArtistMBID == "" && it.Artist != "" && !it.Unlinked {
			for _, name := range []string{it.Artist, FirstArtist(it.Artist)} {
				if m, ok, _ := s.matcher.Known(ctx, "artist", name, ""); ok {
					it.ArtistMBID = m.MBID
					break
				}
			}
		}
		if kind != "songs" && it.MBID == "" && it.ArtistMBID != "" && s.opts.SlimAlbum != nil {
			it.MBID = s.opts.SlimAlbum(ctx, it.ArtistMBID, it.Title)
		}
		dup := k
		if kind != "songs" && it.MBID != "" {
			dup = it.MBID
		}
		if seen[dup] || seen[k] {
			continue
		}
		seen[dup], seen[k] = true, true
		it.PreOrder = len(it.Date) >= 10 && it.Date[:10] > today
		out = append(out, it)
	}
	if len(out) < n {
		for i := range out {
			out[i].Rank, out[i].P = i+1, Popularity(i+1, len(out))
		}
	}
	return out
}

// editionTailRe is an edition named at a title's end: "Deluxe",
// "25th Anniversary (Edition)", or "– The Paradise Edition".
var editionTailRe = regexp.MustCompile(`(?i)(\s+deluxe|\s+\d+(st|nd|rd|th)\s+anniversary(\s+edition)?|\s+[–—]\s+.*edition)$`)

// editionNameRe is a subtitle after ": " that names an edition ("The
// Encore", "THE ANTHOLOGY", "The Complete Confection"), not an album of
// its own ("Live 2014", "the long pond studio sessions").
var editionNameRe = regexp.MustCompile(`(?i)\b(deluxe|edition|anniversary|expanded|complete|encore|anthology|platinum|collector'?s|special|extended|remaster(ed)?|reissue|version)\b`)

// EditionStem answers the album a charted edition is filed under in
// MusicBrainz: the part before ": " when the rest names an edition
// ("Teenage Dream: The Complete Confection") or the part before ends in
// one ("SOS Deluxe: LANA"), without a trailing edition name ("SOS Deluxe", "Bad 25th
// Anniversary"); "" when the title names no edition. " - " is left alone:
// "Dua Lipa - Live From Mexico" is an album of its own.
func EditionStem(title string) string {
	stem := title
	if i := strings.Index(stem, ": "); i > 0 && (editionNameRe.MatchString(stem[i+2:]) || editionTailRe.MatchString(stem[:i])) {
		stem = stem[:i]
	}
	stem = strings.TrimSpace(editionTailRe.ReplaceAllString(stem, ""))
	if stem == "" || stem == title {
		return ""
	}
	return stem
}

// SlimTitleMatch reports a slim-copy title that names the charted one
// (without case, accents, punctuation or edition tags).
func SlimTitleMatch(slim, charted string) bool {
	want := norm(baseTitle(charted))
	return want != "" && norm(baseTitle(slim)) == want
}
