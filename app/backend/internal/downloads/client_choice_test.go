package downloads

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestOrderByPriorityInTurnsWithoutPausedOrResting(t *testing.T) {
	m := &Manager{}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Minute)
	all := []domain.Downloader{
		{Name: "a", Protocol: "usenet", Priority: 1}, {Name: "b", Protocol: "usenet", Priority: 1},
		{Name: "c", Protocol: "usenet", Priority: 2}, {Name: "paused", Protocol: "usenet", Priority: 2, State: "disabled"},
		{Name: "resting", Protocol: "usenet", Priority: 3, BackoffUntil: &later},
	}
	first, skipped := m.order(all, now)
	second, _ := m.order(all, now)
	third, _ := m.order(all, now)
	if names(first) != "a,b,c" || names(second) != "b,a,c" || names(third) != "a,b,c" {
		t.Fatalf("turns: %s | %s | %s", names(first), names(second), names(third))
	}
	if len(skipped) != 2 || !strings.Contains(skipped[0], "paused") || !strings.Contains(skipped[1], "resting until 12:01 UTC") {
		t.Fatalf("skipped %v", skipped)
	}
	back, _ := m.order(all, later.Add(time.Second))
	if !strings.Contains(names(back), "resting") {
		t.Fatalf("after the rest: %s", names(back))
	}
}

func TestGrabSkipsARestingDownloader(t *testing.T) {
	w, m := setup(t)
	until := w.clock.Add(10 * time.Minute)
	dead := w.addDownloader(t, &domain.Downloader{Name: "Resting", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1",
		Priority: 0, AllowLocal: true, BackoffUntil: &until, FailCount: 2, State: "unreachable"})
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	if d := w.downloader(t, dead); d.FailCount != 2 {
		t.Fatalf("the resting Downloader was asked: %+v", d)
	}
	es, _, _ := w.s.Events(context.Background(), database.EventFilter{Kinds: []string{"download_sent"}})
	if len(es) != 1 || es[0].TitleID == nil || es[0].SlotID == nil || es[0].Data["release"] == nil || es[0].Data["downloader"] == nil {
		t.Fatalf("sent events %+v", es)
	}
}

func TestGrabSaysWhyWhenAllAreResting(t *testing.T) {
	w, m := setup(t)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET state = 'disabled'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err == nil || !strings.Contains(err.Error(), "Lab qBittorrent is paused") || len(w.fake.added) != 0 {
		t.Fatalf("err %v, added %d", err, len(w.fake.added))
	}
}

func TestBackoffDoublesAndTheCheckEndsIt(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	id := w.addDownloader(t, &domain.Downloader{Name: "Flaky", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Priority: 5, AllowLocal: true})
	for i, want := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute} {
		if _, err := m.Check(ctx, w.downloader(t, id)); err == nil {
			t.Fatal("a closed port answered")
		}
		d := w.downloader(t, id)
		if d.State != "unreachable" || d.FailCount != i+1 || d.BackoffUntil == nil || !d.BackoffUntil.Equal(w.clock.Add(want)) || d.LastError == "" {
			t.Fatalf("after failure %d: %+v", i+1, d)
		}
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET url = (SELECT url FROM downloaders WHERE name = 'Lab qBittorrent') WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m.Forget(id)
	if _, err := m.Check(ctx, w.downloader(t, id)); err != nil {
		t.Fatal(err)
	}
	if d := w.downloader(t, id); d.State != "ok" || d.FailCount != 0 || d.BackoffUntil != nil || d.LastError != "" || d.CheckedAt == nil {
		t.Fatalf("after recovery %+v", d)
	}
	w.fake.refuse = true
	m.Forget(1)
	if _, err := m.Check(ctx, w.downloader(t, 1)); !errors.Is(err, ErrAuth) || w.downloader(t, 1).State != "auth_failed" {
		t.Fatalf("auth: %v %s", err, w.downloader(t, 1).State)
	}
}

func TestCheckAllSkipsPausedAndRemoved(t *testing.T) {
	w, m := setup(t)
	p := w.addDownloader(t, &domain.Downloader{Name: "Paused", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", State: "disabled", AllowLocal: true})
	r := w.addDownloader(t, &domain.Downloader{Name: "Gone", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Removed: true, AllowLocal: true})
	m.CheckAll(context.Background())
	if w.downloader(t, p).FailCount != 0 || w.downloader(t, r).FailCount != 0 || w.downloader(t, 1).CheckedAt == nil {
		t.Fatal("the status check asked a paused or removed Downloader, or skipped a working one")
	}
}

func TestSpacingPerHost(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	for _, u := range []string{"http://10.0.0.5:8080", "http://10.0.0.5:8090", "http://10.0.0.6:8080"} {
		done, err := m.space(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		done()
	}
	if len(w.slept) != 1 || w.slept[0] != 2*time.Second {
		t.Fatalf("waits %v: the second send to 10.0.0.5 should wait 2 s, the other host none", w.slept)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE settings SET value = '0' WHERE grp = 'links' AND key = ?", SettingSpacing)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	done, _ := m.space(ctx, "http://10.0.0.5:8080")
	done()
	if len(w.slept) != 1 {
		t.Fatalf("waited with spacing off: %v", w.slept)
	}
}

func TestGrabsToOneHostAreSpaced(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	var slot2, rel2 int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var sl mediafiles.Slot
		if err := database.Get(ctx, w.s.Read(), "slots", w.slotID, &sl); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: 1, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		s2 := &mediafiles.Slot{ItemID: it.ID, VersionID: sl.VersionID, State: mediafiles.SlotChosen}
		if _, err := database.Insert(tx, "slots", s2, domain.System); err != nil {
			return err
		}
		r := &domain.Release{SourceID: 1, Name: "Other.2024.1080p.WEB.H264-HOLYARRS", LinkRef: "http://fake-indexer/other.torrent", Protocol: "torrent",
			Size: 1, InfoHash: ptr("1111111111111111111111111111111111111111"), Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{s2.ID},
			Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", r.ID, s2.ID); err != nil {
			return err
		}
		slot2, rel2 = s2.ID, r.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Grab(ctx, slot2, rel2, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.slept) != 1 || w.slept[0] != 2*time.Second || len(w.fake.added) != 2 {
		t.Fatalf("waits %v, added %d", w.slept, len(w.fake.added))
	}
}

func TestDownloaderOffTheLocalNetworkUnlessAllowed(t *testing.T) {
	w, m := setup(t)
	id := w.addDownloader(t, &domain.Downloader{Name: "Strict", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Priority: 9})
	_, err := m.Check(context.Background(), w.downloader(t, id))
	if err == nil || !strings.Contains(err.Error(), "refused by the network policy") {
		t.Fatalf("a Downloader without the local-network switch reached 127.0.0.1: %v", err)
	}
}

// SABnzbd end to end through the manager, on recorded replies.
func TestSabnzbdThroughTheManager(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	f := &fakeSab{}
	u := startSab(t, f)
	m.FromVault = func(context.Context, int64) (string, error) { return sabKey, nil }
	var ref int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO secrets (kind, value, created_at, created_by, updated_at, updated_by) VALUES ('downloader_key', 'sealed', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		ref, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.addDownloader(t, &domain.Downloader{Name: "Lab SABnzbd", Adapter: "sabnzbd", Protocol: "usenet", URL: u, Category: "movies", Priority: 1, AllowLocal: true, SecretRef: &ref})
	var relID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		r := &domain.Release{SourceID: 1, Name: "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS", LinkRef: "http://fake-indexer/good.nzb", Protocol: "usenet",
			Size: 1238952, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{w.slotID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		relID = r.ID
		_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", r.ID, w.slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(ctx, w.slotID, relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.added) != 1 || f.added[0]["cat"] != "movies" || len(w.fake.added) != 0 {
		t.Fatalf("sab added %+v, qbit added %d", f.added, len(w.fake.added))
	}
	if d := w.download(t, id); d.ExternalID != sabGood || d.State != "sent" {
		t.Fatalf("download %+v", d)
	}
	f.queue = "queue-downloading.json"
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("want downloading, got %s", d.State)
	}
	f.queue, f.history = "", "history.json"
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	d := w.download(t, id)
	if d.State != "finished" || d.SavePath != "/data/downloads/usenet/complete/movies/Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS.1" ||
		len(d.FilesSeen) != 1 || d.FilesSeen[0].Name != "Lab.Usenet.Movie.2024.1080p.WEB.H264-HOLYARRS.mkv" {
		t.Fatalf("finished %+v", d)
	}
	if len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("OnFinished %v", w.finshed)
	}
}

func (w *world) addDownloader(t *testing.T, d *domain.Downloader) int64 {
	t.Helper()
	if d.PathMap == nil {
		d.PathMap = []domain.PathMap{}
	}
	if d.AfterImport == "" {
		d.AfterImport = "leave"
	}
	if d.State == "" {
		d.State = "ok"
	}
	if d.Category == "" {
		d.Category = "holyarrs"
	}
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := database.Insert(tx, "downloaders", d, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	return d.ID
}

func (w *world) downloader(t *testing.T, id int64) domain.Downloader {
	t.Helper()
	var d domain.Downloader
	if err := database.Get(context.Background(), w.s.Read(), "downloaders", id, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func names(ds []domain.Downloader) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return strings.Join(out, ",")
}
