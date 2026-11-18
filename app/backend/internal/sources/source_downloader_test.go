package sources

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestSourceKeepsItsChosenDownloader(t *testing.T) {
	ctx := context.Background()
	s, v := openStore(t)
	by := domain.System
	src, err := Add(ctx, s, v, Input{Name: "Private", Format: "torznab", URL: "http://private.example/api"}, by)
	if err != nil {
		t.Fatal(err)
	}
	if src.DownloaderID != nil || SourceViewOf(src, Summary{}).DownloaderID != 0 {
		t.Fatal("a new Source should send to any Downloader")
	}
	seven := int64(7)
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Private", DownloaderID: &seven}, by); err != nil || src.DownloaderID == nil || *src.DownloaderID != 7 {
		t.Fatalf("set: %v %v", src.DownloaderID, err)
	}
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Private"}, by); err != nil || src.DownloaderID == nil || *src.DownloaderID != 7 {
		t.Fatalf("a save without the field should keep it: %v %v", src.DownloaderID, err)
	}
	if SourceViewOf(src, Summary{}).DownloaderID != 7 {
		t.Fatal("the view should show the chosen Downloader")
	}
	zero := int64(0)
	if src, err = Change(ctx, s, v, src.ID, Input{Name: "Private", DownloaderID: &zero}, by); err != nil || src.DownloaderID != nil {
		t.Fatalf("0 should go back to any: %v %v", src.DownloaderID, err)
	}
}
