package downloads

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A grab that ends without a send tells open pages, with the reason, so the
// person who searched sees why nothing was sent.
func TestGrabNotSentTellsOpenPages(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"no Downloader for the protocol", `UPDATE downloaders SET removed = 1`},
		{"no Downloader for the tags", `UPDATE downloaders SET tags = '["4k"]'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, m := setup(t)
			ctx := context.Background()
			if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(tc.sql); return err }); err != nil {
				t.Fatal(err)
			}
			run := &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID)}, Attempt: 1, MaxAttempts: 5}}
			if err := m.HandleGrab(ctx, run); err != nil {
				t.Fatal(err)
			}
			w.mu.Lock()
			defer w.mu.Unlock()
			for i, topic := range w.topics {
				d, _ := w.sent[i].(map[string]any)
				if topic == "slot" && d["id"] == w.slotID && d["not_sent"] == true && d["reason"] != "" {
					return
				}
			}
			t.Fatalf("no not-sent slot event: %v %v", w.topics, w.sent)
		})
	}
}
