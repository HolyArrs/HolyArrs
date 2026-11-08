package downloads

import "testing"

// Totals count the upload speed of seeding rows too (split from the api
// view test).
func TestViewTotalsIncludeSeeding(t *testing.T) {
	if got := viewTotals([]DownloadRow{{DownloadView: DownloadView{State: "downloading", Active: true, Speed: 5, UpSpeed: 1}}, {DownloadView: DownloadView{State: "seeding", UpSpeed: 7}}}); got.Speed != 5 || got.UpSpeed != 8 {
		t.Fatalf("viewTotals %+v", got)
	}
}
