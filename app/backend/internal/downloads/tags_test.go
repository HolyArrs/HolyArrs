package downloads

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestGrabGoesToTheDownloaderSharingTheTitlesTag(t *testing.T) {
	w, m := setup(t)
	lab := w.downloader(t, 1)
	kids := w.addDownloader(t, &domain.Downloader{Name: "Kids qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: lab.URL,
		Priority: 9, AllowLocal: true, Tags: []string{"kids"}})
	tagTitle(t, w, `["kids"]`)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != kids {
		t.Fatalf("sent to Downloader %d, want the tagged one %d (the untagged one has the better priority)", d.DownloaderID, kids)
	}
}

func TestGrabOfAnUntaggedTitleSkipsTaggedDownloaders(t *testing.T) {
	w, m := setup(t)
	lab := w.downloader(t, 1)
	w.addDownloader(t, &domain.Downloader{Name: "Kids qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: lab.URL,
		Priority: 0, AllowLocal: true, Tags: []string{"kids"}})
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.DownloaderID != lab.ID {
		t.Fatalf("sent to Downloader %d, want the untagged %d", d.DownloaderID, lab.ID)
	}
}

func TestGrabWithNoDownloaderForTheTagsWaitsOnADecision(t *testing.T) {
	w, m := setup(t)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE downloaders SET tags = '["4k"]'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	tagTitle(t, w, `["anime"]`)
	_, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	var te *TagsError
	if !errors.As(err, &te) || !strings.Contains(err.Error(), "(anime)") {
		t.Fatalf("Grab: %v", err)
	}
	run := &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "slot", ID: w.slotID}, Params: map[string]any{"release_id": float64(w.relID)}}}
	if err := m.HandleGrab(context.Background(), run); err != nil {
		t.Fatalf("the Job should end, not retry: %v", err)
	}
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), w.s.Read(), "slots", w.slotID, &sl); err != nil {
		t.Fatal(err)
	}
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || !strings.HasPrefix(*sl.LastReason, "not sent: no torrent Downloader has this title's tags (anime)") {
		t.Fatalf("slot %s, reason %v", sl.State, sl.LastReason)
	}
	var n int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM downloads").Scan(&n)
	if n != 0 || len(w.fake.added) != 0 {
		t.Fatalf("something was sent: %d downloads", n)
	}
}

func tagTitle(t *testing.T, w *world, tags string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET tags = ?", tags)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
