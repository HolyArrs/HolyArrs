package discover

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// spanishText keeps a Spanish name for every title.
type spanishText struct{ *metadata.Static }

func (spanishText) KeptText(_ context.Context, _, id, lang string) (metadata.TitleText, bool) {
	if lang != "es" {
		return metadata.TitleText{}, true
	}
	return metadata.TitleText{Name: "Título " + id}, true
}

// Browse grids and the New & Popular day rail show names in the person's
// language, like every other Explore row.
func TestRowsTextInPersonsLanguage(t *testing.T) {
	h := &Handlers{Metadata: spanishText{metadata.Lab()}}
	r := httptest.NewRequest("GET", "/api/v1/explore/browse/category", nil)
	r.Header.Set("X-HolyArrs-Language", "es")
	views := []ExploreRowView{{Items: []ItemView{{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "7", Name: "Seven"}}}}}
	h.rowsText(r, views)
	if got := views[0].Items[0].Name; got != "Título 7" {
		t.Fatalf("name %q", got)
	}
}
