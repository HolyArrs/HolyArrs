package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestNZBGetLab is opt-in. The acceptance runner uses add, check and remove
// in separate processes so it can restart the disposable app between them.
func TestNZBGetLab(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_NZBGET_URL")
	mode := os.Getenv("HOLYARRS_LAB_NZBGET_MODE")
	if base == "" || mode == "" {
		t.Skip("set HOLYARRS_LAB_NZBGET_URL and HOLYARRS_LAB_NZBGET_MODE")
	}
	password := os.Getenv("HOLYARRS_LAB_NZBGET_PASS")
	if path := os.Getenv("HOLYARRS_LAB_NZBGET_PASS_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil || len(b) == 0 || len(b) > 4096 {
			t.Fatal("read disposable NZBGet password file failed")
		}
		password = string(b)
	}
	n := NewNZBGet(base, os.Getenv("HOLYARRS_LAB_NZBGET_USER"), password, nil)
	n.Category = "holyarrs"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := n.Health(ctx)
	if err != nil || h.Version == "" {
		t.Fatalf("health %+v: %v", h, err)
	}
	fmt.Printf("LAB_NZBGET_VERSION=%s\n", h.Version)

	switch mode {
	case "add":
		marker := os.Getenv("HOLYARRS_LAB_NZBGET_MARKER")
		if marker == "" {
			t.Fatal("set HOLYARRS_LAB_NZBGET_MARKER")
		}
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <file poster="holyarrs@example.invalid" date="1" subject="HolyArrs disposable acceptance">
    <groups><group>alt.binaries.test</group></groups>
    <segments><segment bytes="16" number="1">holyarrs-acceptance@example.invalid</segment></segments>
  </file>
</nzb>`))
		}))
		defer source.Close()
		id, err := n.Add(ctx, AddRequest{URL: source.URL + "/acceptance.nzb", Name: "HolyArrs.NZBGet.Acceptance", Category: "holyarrs",
			AttemptMarker: marker, AllowLocal: true})
		if err != nil || id == "" {
			t.Fatalf("add %q: %v", id, err)
		}
		if _, err := n.Get(ctx, id); err != nil {
			active, history, rowsErr := n.rows(ctx)
			t.Logf("disposable rows active=%+v history=%+v err=%v", active, history, rowsErr)
			t.Fatalf("get after add: %v", err)
		}
		fmt.Printf("LAB_NZBGET_ID=%s\n", id)
	case "reconcile":
		marker := os.Getenv("HOLYARRS_LAB_NZBGET_MARKER")
		id, err := n.Add(ctx, AddRequest{Name: "HolyArrs.NZBGet.Acceptance", Category: "holyarrs",
			AttemptMarker: marker, ReconcileOnly: true})
		if err != nil || id != marker {
			t.Fatalf("reconcile %q: %v", id, err)
		}
		fmt.Printf("LAB_NZBGET_RECONCILED_ID=%s\n", id)
	case "check":
		id := os.Getenv("HOLYARRS_LAB_NZBGET_ID")
		j, err := n.Get(ctx, id)
		if err != nil {
			t.Fatalf("get after restart: %v", err)
		}
		fmt.Printf("LAB_NZBGET_STATE=%s DETAIL=%s\n", j.State, j.Detail)
	case "history":
		id := os.Getenv("HOLYARRS_LAB_NZBGET_ID")
		active, _, err := n.rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		native := 0
		for _, row := range active {
			if nzbgetMarkerID(row.Parameters) == id {
				native = row.NZBID
			}
		}
		if native == 0 {
			t.Fatal("owned active row was not found")
		}
		var ok bool
		if err := n.rpc(ctx, "editqueue", []any{"GroupDelete", "", []int{native}}, &ok); err != nil || !ok {
			t.Fatalf("move to history: %v ok=%v", err, ok)
		}
		var j Job
		for i := 0; i < 20; i++ {
			j, err = n.Get(ctx, id)
			if err == nil && j.State == StateFailed {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil || j.State != StateFailed {
			t.Fatalf("history marker %+v: %v", j, err)
		}
		fmt.Printf("LAB_NZBGET_HISTORY_STATE=%s DETAIL=%s\n", j.State, j.Detail)
	case "remove":
		id := os.Getenv("HOLYARRS_LAB_NZBGET_ID")
		// Keeping files is a park-delete: the job leaves the queue for the finished
		// list, then the files-too remove clears it from there as well.
		if err := n.Remove(ctx, id, false); err != nil {
			t.Fatalf("remove keeping files: %v", err)
		}
		// A finished job removed keeping its files is hidden from the list at once,
		// so the files-too remove may find nothing left to do.
		if err := n.Remove(ctx, id, true); err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatalf("remove: %v", err)
		}
		if _, err := n.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get after remove: %v", err)
		}
	default:
		t.Fatalf("unknown mode %q", mode)
	}
}
