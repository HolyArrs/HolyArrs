package api

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// libraryBioMax is the most of a bio the Library list carries: the Overview
// view shows two or three lines of it.
const libraryBioMax = 320

// keptArtistBio is the start of the artist's Wikipedia summary as the music
// pages already keep it (the metadata server's by MusicBrainz id, else the
// stand-in's by name). It only reads what is stored: never an outside call.
func (a *API) keptArtistBio(ctx context.Context, mbid, name string) string {
	keys := []string{}
	if mbid != "" {
		keys = append(keys, "bio2:"+strings.ToLower(mbid))
	}
	if n := strings.TrimSpace(name); n != "" {
		keys = append(keys, "bio:"+strings.ToLower(n))
	}
	for _, k := range keys {
		row, ok := a.sideRead(ctx, k)
		if !ok || row.Miss || len(row.V) == 0 {
			continue
		}
		var b *metadata.WikiSummary
		if json.Unmarshal(row.V, &b) != nil || b == nil {
			continue
		}
		if s := shortBio(b.Extract); s != "" {
			return s
		}
	}
	return ""
}

// shortBio cuts a summary to whole sentences within libraryBioMax bytes,
// or to a word with an ellipsis when the first sentence is longer.
func shortBio(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= libraryBioMax {
		return s
	}
	cut := s[:libraryBioMax]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	if i := strings.LastIndex(cut, ". "); i > libraryBioMax/3 {
		return cut[:i+1]
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, ",;:") + "…"
}
