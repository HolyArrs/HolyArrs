package discover

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// FillTrailerKeys sets trailer_key on Explore cards whose key is kept.
func FillTrailerKeys(r *http.Request, items []ItemView, kid bool, warmTitles func(lang string, titles [][2]string), knownKey func(ctx context.Context, kind, id, lang string) string) {
	lang := metadata.TrailerLanguage(r)
	// Every listed title is warmed ahead (details, extras, trailer key), so
	// opening or hovering it never makes the first outside call.
	warm := make([][2]string, 0, len(items))
	for _, v := range items {
		if v.Kind == "movie" || v.Kind == "show" {
			warm = append(warm, [2]string{v.Kind, v.TMDBID})
		}
	}
	warmTitles(lang, warm)
	if kid {
		return
	}
	for i := range items {
		if v := &items[i]; (v.Kind == "movie" || v.Kind == "show") && v.TMDBID != "" {
			v.TrailerKey = knownKey(r.Context(), v.Kind, v.TMDBID, lang)
		}
	}
}

// TrailerPassLanguages are the languages pages asked for lately (most card
// pictures kept first, then the latest asked), at most metadata.TrailerPassLangs.
// Before any page asked (just after a start) the card pictures' languages
// count instead; "en" when none.
// at holds the languages pages asked for lately, with when.
func TrailerPassLanguages(ctx context.Context, store *database.Store, at map[string]time.Time) []string {
	var asked []string
	for l := range at {
		asked = append(asked, l)
	}
	slices.SortFunc(asked, func(x, y string) int { return at[y].Compare(at[x]) })
	var out []string
	add := func(l string) {
		if len(out) < metadata.TrailerPassLangs && len(l) == 2 && metadata.ValidTrailerLanguage(l) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if rows, err := store.Read().QueryContext(ctx, "SELECT lang FROM card_art GROUP BY lang ORDER BY count(*) DESC, lang LIMIT 20"); err == nil {
		for rows.Next() {
			var l string
			// The filler keeps English pictures for every title; they count
			// only when a page asked for English.
			if rows.Scan(&l) == nil && (len(asked) == 0 || at[l] != (time.Time{})) {
				add(l)
			}
		}
		rows.Close()
	}
	for _, l := range asked {
		add(l)
	}
	if len(out) == 0 {
		out = []string{"en"}
	}
	return out
}
