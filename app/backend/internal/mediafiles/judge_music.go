package mediafiles

import (
	"fmt"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// hintsFor reads stored names as music when the Pick rules judge music.
func hintsFor(rules PickRulesBody) releasename.Hints {
	return releasename.Hints{Music: IsMusicRules(rules)}
}

// musicStep maps what a music name says to a music ladder step
// (pick-rules.md section 8). "" when the name gives no format or bit rate.
func musicStep(m *releasename.MusicInfo) string {
	if m.Lossless {
		if m.BitDepth >= 24 || m.SampleRate > 48 || m.Format == "dsd" {
			return "lossless_hires"
		}
		return "lossless"
	}
	switch m.VBR {
	case "V0":
		return "lossy_high"
	case "V1", "V2":
		return "lossy_mid"
	}
	high := 320
	switch m.Format {
	case "aac", "vorbis":
		high = 256
	case "opus":
		high = 160
	}
	switch {
	case m.Bitrate >= high:
		return "lossy_high"
	case m.Bitrate >= 192:
		return "lossy_mid"
	case m.Bitrate > 0:
		return "lossy_low"
	case m.Format != "" || m.VBR == "VBR":
		// A lossy format with no bit rate: read as the middle step.
		return "lossy_mid"
	case m.BitDepth >= 24:
		return "lossless_hires"
	}
	return ""
}

// musicTrait says whether a music Trait matches.
func musicTrait(trait string, p releasename.Result) (bool, string) {
	m := p.Music
	switch trait {
	case "cd_source":
		return m.Source == "cd", "ripped from a CD"
	case "web_source":
		return m.Source == "web", "bought from a web shop"
	case "vinyl_source":
		return m.Source == "vinyl", "recorded from vinyl"
	case "log_cue":
		return p.Has("log") || p.Has("cue"), "comes with a rip log or cue sheet"
	case "hires":
		return m.BitDepth >= 24, "24-bit"
	case "mp3", "aac", "opus", "vorbis", "flac", "alac":
		return m.Format == trait, "audio is " + strings.ToUpper(trait)
	case "single_image":
		return p.Has("single_image"), "the whole album in one file with a cue sheet"
	case "lossy_transcode":
		return p.Has("transcode"), "says it is a transcode"
	case "deluxe":
		return hasEdition(m, "deluxe") || hasEdition(m, "expanded"), "a deluxe or expanded edition"
	}
	return false, "unknown trait " + trait
}

func hasEdition(m *releasename.MusicInfo, e string) bool {
	for _, x := range m.Editions {
		if x == e {
			return true
		}
	}
	return false
}

func mbOf(n int64) float64 { return float64(n) / float64(1<<20) }

// musicSize checks an album's size against the step's band (per minute of
// the tracks it fills).
func (v *Verdict) musicSize(size, lo, hi int64) {
	v.sizeOff = size - sizeMidpoint(lo, hi)
	if v.sizeOff < 0 {
		v.sizeOff = -v.sizeOff
	}
	switch {
	case size < lo:
		v.add("hard", "size", "fail", fmt.Sprintf("%.0f MB is under the %.0f MB minimum for this album", mbOf(size), mbOf(lo)), 0)
	case hi > 0 && size > hi:
		v.add("hard", "size", "fail", fmt.Sprintf("%.0f MB is over the %.0f MB maximum for this album", mbOf(size), mbOf(hi)), 0)
	default:
		v.add("hard", "size", "pass", fmt.Sprintf("%.0f MB within %.0f to %.0f MB", mbOf(size), mbOf(lo), mbOf(hi)), 0)
	}
}

// matchArtist finds the artist a music name names, by clean name or alias
// (the alternative readings of a scene name too).
func matchArtist(p releasename.Result, lib []TitleRef) Match {
	m := p.Music
	names := []string{m.Artist}
	for _, a := range m.Alts {
		names = append(names, a.Artist)
	}
	for _, n := range names {
		want := releasename.CleanName(n)
		if want == "" {
			continue
		}
		for i := range lib {
			t := &lib[i]
			if t.Kind != "artist" {
				continue
			}
			if releasename.CleanName(t.Name) == want {
				return Match{Title: t, Conf: releasename.Sure, Detail: t.Name + " by clean name"}
			}
			for _, a := range t.Aliases {
				if releasename.CleanName(a) == want {
					return Match{Title: t, Conf: releasename.Sure, Detail: fmt.Sprintf("%s by its other name %s", t.Name, a)}
				}
			}
		}
	}
	// No clear split ("Artist Album 2019 FLAC"): the words may start with
	// an artist's name.
	if m.Conf == releasename.Guess {
		whole := releasename.CleanName(m.Artist)
		for i := range lib {
			t := &lib[i]
			if n := releasename.CleanName(t.Name); t.Kind == "artist" && n != "" && strings.HasPrefix(whole+" ", n+" ") {
				return Match{Title: t, Conf: releasename.Sure, Detail: t.Name + " by the start of the name"}
			}
		}
	}
	if m.Various {
		return Match{Conf: releasename.Unknown, Detail: "a various artists Release; it names no single artist"}
	}
	return Match{Conf: releasename.Unknown, Detail: fmt.Sprintf("no artist in the library is called %q", m.Artist)}
}

// AlbumRef is one album of an artist as the matcher sees it.
type AlbumRef struct {
	PartID int64
	Name   string
	Year   int // 0: no release date
}

// AlbumName is the album a music name holds for a matched artist: the
// album part, or the words after the artist's name when the name had no
// clear split; the alternative readings are tried too.
func albumNames(p releasename.Result, artist string) []string {
	m := p.Music
	out := []string{m.Album}
	for _, a := range m.Alts {
		if releasename.CleanName(a.Artist) == releasename.CleanName(artist) {
			out = append(out, a.Album)
		}
	}
	if m.Conf == releasename.Guess {
		whole, n := releasename.CleanName(m.Artist), releasename.CleanName(artist)
		if rest, ok := strings.CutPrefix(whole, n+" "); ok {
			out = append(out, rest)
		}
	}
	return out
}

// MatchAlbum finds the album of the matched artist a music name holds. A
// year tells same-named albums apart; a different year alone (a later
// remaster) only warns. It returns the album, a warning and, when none
// matches, why.
func MatchAlbum(p releasename.Result, artist string, albums []AlbumRef) (*AlbumRef, string, string) {
	if p.Music == nil {
		return nil, "", "not a music name"
	}
	var cands []*AlbumRef
	for _, n := range albumNames(p, artist) {
		want := releasename.CleanName(n)
		if want == "" {
			continue
		}
		for i := range albums {
			a := &albums[i]
			if releasename.CleanName(a.Name) == want {
				cands = append(cands, a)
			}
		}
		if len(cands) > 0 {
			break
		}
	}
	y := p.Year.Value
	switch {
	case len(cands) == 0 && p.Music.Album == "":
		return nil, "", "the name gives no album"
	case len(cands) == 0:
		return nil, "", fmt.Sprintf("%s has no album called %q", artist, p.Music.Album)
	case len(cands) == 1:
		a := cands[0]
		if y > 0 && a.Year > 0 && y != a.Year {
			return a, fmt.Sprintf("the name says %d; %s came out in %d (a reissue or remaster?)", y, a.Name, a.Year), ""
		}
		return a, "", ""
	}
	// Same-named albums: the year must leave exactly one; two that share it
	// (a studio album and a live one released the same year) are refused.
	pick := func(ok func(a *AlbumRef) bool) ([]*AlbumRef, bool) {
		var out []*AlbumRef
		for _, a := range cands {
			if y > 0 && ok(a) {
				out = append(out, a)
			}
		}
		return out, len(out) > 0
	}
	if same, ok := pick(func(a *AlbumRef) bool { return a.Year == y }); ok {
		if len(same) > 1 {
			return nil, "", fmt.Sprintf("%d albums are called %q and came out in %d; the name does not tell them apart", len(same), p.Music.Album, y)
		}
		return same[0], "", ""
	}
	if near, ok := pick(func(a *AlbumRef) bool { d := a.Year - y; return d == 1 || d == -1 }); ok {
		if len(near) > 1 {
			return nil, "", fmt.Sprintf("%d albums are called %q and came out a year from %d; the name does not tell them apart", len(near), p.Music.Album, y)
		}
		a := near[0]
		return a, fmt.Sprintf("the name says %d; %s came out in %d", y, a.Name, a.Year), ""
	}
	return nil, "", fmt.Sprintf("%d albums are called %q and the name gives no year that tells them apart", len(cands), p.Music.Album)
}
