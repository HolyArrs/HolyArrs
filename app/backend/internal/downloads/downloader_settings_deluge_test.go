package downloads

import (
	"errors"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestDelugeCategoryMustFitLabelPlugin(t *testing.T) {
	for c, want := range map[string]error{"movies": nil, "tv-shows_4k.x": nil, "Movies": ErrDelugeLabel, "tv shows": ErrDelugeLabel} {
		cat := c
		var d domain.Downloader
		err := fill(Input{Name: "d", Adapter: "deluge", URL: "http://deluge:8112", Category: &cat}, &d, true)
		if !errors.Is(err, want) && !(want == nil && err != nil && !errors.Is(err, ErrDelugeLabel)) {
			t.Fatalf("%q: %v", c, err)
		}
	}
	cat := "Movies"
	var d domain.Downloader
	if err := fill(Input{Name: "q", Adapter: "qbittorrent", URL: "http://q:8080", Category: &cat}, &d, true); errors.Is(err, ErrDelugeLabel) {
		t.Fatal("qBittorrent should keep mixed case")
	}
}
