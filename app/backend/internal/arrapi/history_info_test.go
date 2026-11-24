package arrapi

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestHistoryGrabCarriesReleasePage(t *testing.T) {
	d := map[string]any{}
	historyData(d, database.Event{Kind: "grabbed"}, "Show.S01E01.1080p.WEB-GRP", historyRelease{info: "https://site.example/details/1"})
	if d["nzbInfoUrl"] != "https://site.example/details/1" {
		t.Errorf("nzbInfoUrl = %v", d["nzbInfoUrl"])
	}
	d = map[string]any{}
	historyData(d, database.Event{Kind: "grabbed"}, "x", historyRelease{})
	if _, ok := d["nzbInfoUrl"]; ok {
		t.Error("no page must leave nzbInfoUrl out, as the old apps do")
	}
}
