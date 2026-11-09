//go:build lab

package downloads

import (
	"context"
	"encoding/base64"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestLabNZBGetParity(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_NZBGET_URL")
	if base == "" {
		t.Skip("HOLYARRS_LAB_NZBGET_URL not set")
	}
	n := NewNZBGet(base, os.Getenv("HOLYARRS_LAB_NZBGET_USER"), os.Getenv("HOLYARRS_LAB_NZBGET_PASS"), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	nzb := `<?xml version="1.0" encoding="utf-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"><file poster="lab@example.invalid" date="1" subject="HolyArrs parity &quot;parity.bin&quot; yEnc (1/1)">
<groups><group>alt.binaries.test</group></groups><segments><segment bytes="1000" number="1">parity-` + strconv.FormatInt(time.Now().UnixNano(), 36) + `@example.invalid</segment></segments></file></nzb>`
	var nzbID int
	if err := n.rpc(ctx, "append", []any{"HolyArrs.Parity.Lab.nzb", base64.StdEncoding.EncodeToString([]byte(nzb)), "tv", 0, false, true, "", 0, "ALL", false, [][]string{}}, &nzbID); err != nil || nzbID <= 0 {
		t.Fatalf("append %d %v", nzbID, err)
	}
	id := "nzbid-" + strconv.Itoa(nzbID)
	t.Cleanup(func() {
		var ok bool
		_ = n.rpc(context.Background(), "editqueue", []any{"GroupFinalDelete", "", []int{nzbID}}, &ok)
		_ = n.rpc(context.Background(), "editqueue", []any{"HistoryFinalDelete", "", []int{nzbID}}, &ok)
	})
	q, err := n.Queue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got *Job
	for i := range q.Jobs {
		if q.Jobs[i].ID == id {
			got = &q.Jobs[i]
		}
	}
	if got == nil || got.State != StatePaused || got.Category != "tv" || got.Size <= 0 {
		t.Fatalf("queue %+v", q.Jobs)
	}
	if _, err := n.Status(ctx); err != nil {
		t.Fatal(err)
	}
	var ok bool
	if err := n.rpc(ctx, "editqueue", []any{"GroupDelete", "", []int{nzbID}}, &ok); err != nil || !ok {
		t.Fatalf("delete into history %v", err)
	}
	var j Job
	for range 20 {
		if j, err = n.Get(ctx, id); err == nil && j.State == StateFailed {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if j.State != StateFailed || j.Detail == "" {
		t.Fatalf("history job %+v %v", j, err)
	}
	t.Logf("failed reason: %s", j.Detail)
	st, err := n.JobStages(ctx, id)
	if err != nil || len(st) == 0 {
		t.Fatalf("stages %v %+v", err, st)
	}
	h, err := n.History(ctx, 0)
	if err != nil || !h.Changed {
		t.Fatal(err)
	}
	if nid, err := n.Retry(ctx, id); err != nil || nid != id {
		t.Fatalf("retry %s %v", nid, err)
	}
	for range 20 {
		if j, err = n.Get(ctx, id); err == nil && j.State != StateFailed {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if j.State == StateFailed {
		t.Fatalf("after retry %+v", j)
	}
	if err := n.Remove(ctx, id, true); err != nil {
		t.Fatal(err)
	}
}
