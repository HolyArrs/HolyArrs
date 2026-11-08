package downloads

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestTransmissionDisposableApp(t *testing.T) {
	base := os.Getenv("HOLYARRS_TRANSMISSION_LAB_URL")
	if base == "" {
		t.Skip("disposable Transmission not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e := NewTransmission(base, "", "", Client(base, true))
	e.category = "holyarrs-disposable"
	h, err := e.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	piece := sha1.Sum([]byte("x"))
	b := []byte(fmt.Sprintf("d4:infod6:lengthi1e4:name9:owned.txt12:piece lengthi16384e6:pieces20:%see", piece[:]))
	id, err := torrentFileHash(b)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(b) }))
	defer srv.Close()
	if os.Getenv("HOLYARRS_TRANSMISSION_LAB_PHASE") == "add" {
		r := AddRequest{URL: srv.URL + "/owned.torrent", Name: "Owned fixture", Category: e.category, SavePath: "/downloads", InfoHash: id, AllowLocal: true}
		got, err := e.Add(ctx, r)
		if err != nil || got != id {
			t.Fatalf("add: id matches=%t error=%v", got == id, err)
		}
		again, err := e.Add(ctx, r)
		if err != nil || again != id {
			t.Fatalf("repeat add: %v", err)
		}
	}
	var job Job
	for {
		job, err = e.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == StateFinished {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("completion state=%s progress=%v", job.State, job.Progress)
		case <-time.After(200 * time.Millisecond):
		}
	}
	list, err := e.List(ctx, e.category)
	if err != nil || len(list) != 1 || len(job.Files) != 1 {
		t.Fatalf("list/files count=%d/%d error=%v", len(list), len(job.Files), err)
	}
	other, err := e.List(ctx, "foreign")
	if err != nil || len(other) != 0 {
		t.Fatal("category isolation failed")
	}
	if got, err := e.Add(ctx, AddRequest{InfoHash: id, Category: e.category, ReconcileOnly: true}); err != nil || got != id {
		t.Fatal("saved-hash reconciliation failed")
	}
	if os.Getenv("HOLYARRS_TRANSMISSION_LAB_PHASE") == "remove" {
		if err := e.Remove(ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("version=%s json_rpc_2=%t hash=%s state=%s files=%d phase=%s", h.Version, e.modern, id, job.State, len(job.Files), os.Getenv("HOLYARRS_TRANSMISSION_LAB_PHASE"))
}
