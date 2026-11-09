package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// route: POST /api/v1/downloaders/{id}/items/{hash}/up
// route: POST /api/v1/downloaders/{id}/items/{hash}/down
// route: POST /api/v1/downloaders/{id}/items/{hash}/bottom
// route: POST /api/v1/downloaders/{id}/items/{hash}/priority
func TestUsenetQueueRoutesOnATorrentClient(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	base := x.srv.URL + "/api/v1/downloaders/" + dlItoa(did) + "/items/" + outHash
	for _, act := range []string{"up", "down", "bottom"} {
		do(t, "POST", base+"/"+act, "", 409)
	}
	do(t, "POST", base+"/priority", `{"priority":"urgent"}`, 400)
	do(t, "POST", base+"/priority", `{"priority":"high"}`, 409)
}
