package discover

import (
	"context"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Title pages and Explore cards show names and synopses in the language
// the page is shown in (metadata.TextLanguage), from the texts kept with
// each title's details. Nothing here asks outside; the Library keeps its
// English record.

// textKept says whether the title's texts were kept with its details; a
// provider without texts counts as kept (English only).
func textKept(ctx context.Context, p metadata.Provider, kind, id string) bool {
	x, ok := metadata.TexterFor(ctx, p)
	if !ok {
		return true
	}
	_, kept := x.KeptText(ctx, kind, id, "en")
	return kept
}

// keptText is the title's text in lang; ok is false while its details were
// not read with texts. English is the record itself.
func keptText(ctx context.Context, p metadata.Provider, kind, id, lang string) (metadata.TitleText, bool) {
	if lang == "en" {
		return metadata.TitleText{}, true
	}
	x, ok := metadata.TexterFor(ctx, p)
	if !ok {
		return metadata.TitleText{}, true
	}
	return x.KeptText(ctx, kind, id, lang)
}

// FillText puts each card's name and synopsis in the asking page's
// language; a card without kept text keeps English.
func FillText(r *http.Request, p metadata.Provider, items []ItemView) {
	lang := metadata.TextLanguage(r)
	if lang == "en" || p == nil {
		return
	}
	for i := range items {
		if v := &items[i]; v.Kind == "movie" || v.Kind == "show" {
			if x, ok := keptText(r.Context(), p, v.Kind, v.TMDBID, lang); ok {
				x.Apply(&v.DiscoverItem)
			}
		}
	}
}

// rowsText puts every row's cards in the asking page's language, for rows
// that skip CardArt.Fill (Browse grids, the New & Popular day rail).
func (h *Handlers) rowsText(r *http.Request, views []ExploreRowView) {
	for i := range views {
		FillText(r, h.Metadata, views[i].Items)
	}
}

// detailsText shows a title page in x: its name, synopsis and tagline, and
// the original title only while it differs from the name shown.
func detailsText(d *metadata.Details, x metadata.TitleText) {
	if x.Name != "" && x.Name != d.Name {
		switch {
		case d.OriginalTitle == "":
			// No original title kept: it is the English name.
			d.OriginalTitle = d.Name
		case strings.EqualFold(d.OriginalTitle, x.Name):
			d.OriginalTitle = ""
		}
	}
	x.Apply(&d.DiscoverItem)
	if x.Tagline != "" {
		d.Tagline = x.Tagline
	}
}

// seasonText shows a season's episodes in texts (by episode number), on a
// copy: the cached English list is never changed.
func seasonText(eps []metadata.DetailEpisode, texts map[int]metadata.TitleText) []metadata.DetailEpisode {
	if len(texts) == 0 {
		return eps
	}
	out := append([]metadata.DetailEpisode(nil), eps...)
	for i := range out {
		x := texts[out[i].Number]
		if x.Name != "" {
			out[i].Name = x.Name
		}
		if x.Overview != "" {
			out[i].Overview = x.Overview
		}
	}
	return out
}
