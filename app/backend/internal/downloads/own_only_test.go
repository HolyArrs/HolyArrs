package downloads

import (
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// With "Show only HolyArrs items" on, another app's torrents on the same
// qBittorrent are hidden and every single or bulk change to them is refused;
// with it off they show and can be changed as before.
func TestOwnOnlyHidesAndRefusesOtherApps(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	pointQB(t, x, qbList(t, outsideTorrents(3, 0)).URL)
	feedStore(t, x.dl)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id)
	theirs := fmt.Sprintf("%040x", 1)
	item := fmt.Sprintf("%s/api/v1/downloaders/%d/items/%s/", x.srv.URL, id, theirs)
	bulk := fmt.Sprintf("%s/api/v1/downloaders/%d/bulk", x.srv.URL, id)
	body := `{"action":"pause","hashes":["` + theirs + `"]}`

	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); v.OutsideTotal != 3 {
		t.Fatalf("switch off: outside %d", v.OutsideTotal)
	}
	do(t, "POST", item+"pause", "", 200)
	do(t, "POST", bulk, body, 200)

	exec41(t, x.s, "UPDATE downloaders SET own_only = 1")
	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); v.OutsideTotal != 0 || len(v.Downloads) != 1 {
		t.Fatalf("switch on: outside %d, rows %d", v.OutsideTotal, len(v.Downloads))
	}
	for _, act := range []string{"pause", "resume", "recheck", "reannounce", "top"} {
		do(t, "POST", item+act, "", 403)
	}
	do(t, "POST", item+"remove", `{"delete_files":false}`, 403)
	do(t, "POST", bulk, body, 403)
	do(t, "POST", bulk, `{"action":"remove","hashes":["`+theirs+`"]}`, 403)
	do(t, "POST", fmt.Sprintf("%s/api/v1/downloaders/%d/speed-limits", x.srv.URL, id), `{"hashes":["`+theirs+`"],"down":0,"up":0}`, 403)
}
