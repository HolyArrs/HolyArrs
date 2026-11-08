package importer

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestFileReplacedKeepsOldAndNewQuality(t *testing.T) {
	w, _, _ := upgradeWorld(t, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		"Dune.Part.Two.2024.720p.HDTV.x264-OTHER")
	ctx := context.Background()
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	es, _, err := w.s.Events(ctx, database.EventFilter{Kinds: []string{"file_replaced"}})
	if err != nil || len(es) != 1 {
		t.Fatalf("events %v %v", es, err)
	}
	d := es[0].Data
	if d["new_quality"] != "1080p WEB-DL" || d["new_release"] != "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS" || d["old_release"] != "Dune.Part.Two.2024.720p.HDTV.x264-OTHER" {
		t.Fatalf("data %v", d)
	}
}

func TestQualityText(t *testing.T) {
	if got := qualityText(mediafiles.Quality{Resolution: 2160, Source: "web_dl", HDR: []string{"dv"}}); got != "2160p WEB-DL DV" {
		t.Fatal(got)
	}
	if got := qualityText(mediafiles.Quality{}); got != "" {
		t.Fatal(got)
	}
}
