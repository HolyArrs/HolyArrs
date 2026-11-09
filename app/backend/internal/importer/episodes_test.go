package importer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestEpisodeDownloadWaitsForEpisodeImport(t *testing.T) {
	defer func(old bool) { canImportEpisodes = old }(canImportEpisodes)
	canImportEpisodes = false
	w := setup(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET kind = 'show'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	if _, err := im.Import(ctx, w.dlID, false, domain.System); !errors.Is(err, ErrEpisodeImport) {
		t.Fatalf("import of a show download: %v", err)
	}
	if entries, _ := os.ReadDir(w.lib); len(entries) != 0 {
		t.Fatal("an episode download was placed as a movie")
	}
	var st string
	_ = w.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", w.slotID).Scan(&st)
	if st != string(mediafiles.SlotDownloading) {
		t.Fatalf("slot moved to %s", st)
	}
}

func TestStrandedShowDownloadsRequeued(t *testing.T) {
	w := setup(t)
	ctx := context.Background()
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET kind = 'show'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	im := &Importer{Store: w.s}
	defer func(old bool) { canImportEpisodes = old }(canImportEpisodes)
	canImportEpisodes = false
	var queued []int64
	enqueue := func(_ context.Context, id int64) { queued = append(queued, id) }
	if n, err := im.RequeueStranded(ctx, enqueue); err != nil || n != 0 || len(queued) != 0 {
		t.Fatalf("before episode import: %d %v %v", n, err, queued)
	}
	canImportEpisodes = true
	if n, err := im.RequeueStranded(ctx, enqueue); err != nil || n != 1 || len(queued) != 1 || queued[0] != w.dlID {
		t.Fatalf("with episode import: %d %v %v", n, err, queued)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'wanted'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	queued = nil
	if n, _ := im.RequeueStranded(ctx, enqueue); n != 0 {
		t.Fatalf("slot moved on: %d", n)
	}
}
