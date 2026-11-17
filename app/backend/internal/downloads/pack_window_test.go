package downloads

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// packFiles is the file list of the lab season pack: four episodes and an nfo.
func packFiles() []map[string]any {
	out := []map[string]any{}
	for i, n := range []string{"E01", "E02", "E03", "E04"} {
		out = append(out, map[string]any{"index": i, "name": "Lab.Show.S01.1080p/Lab.Show.S01" + n + ".1080p.WEB.H264-GRP.mkv", "size": 1000, "progress": 0, "priority": 1})
	}
	return append(out, map[string]any{"index": 4, "name": "Lab.Show.S01.1080p/Lab.Show.S01.nfo", "size": 10, "progress": 0, "priority": 1})
}

// packWindowWorld turns the lab title into a show with four episodes of
// season 1, two ahead, and the release into a season pack chosen for the
// first two. withFiles puts the pack's file list in the fake qBittorrent.
func packWindowWorld(t *testing.T, withFiles bool) (*world, *Manager, []int64) {
	t.Helper()
	w, m := setup(t)
	ctx := context.Background()
	slots := []int64{w.slotID}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var itemID, verID, titleID int64
		if err := tx.QueryRow("SELECT s.item_id, s.version_id, i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", w.slotID).Scan(&itemID, &verID, &titleID); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE titles SET kind = 'show', name = 'Lab Show', pack_window = 2 WHERE id = ?", titleID); err != nil {
			return err
		}
		p := &mediafiles.Part{TitleID: titleID, Kind: "season", Number: ptr(1), Name: "Season 1", KeepUpToDate: true, SourceIDs: map[string]string{},
			Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE items SET kind = 'episode', part_id = ?, numbers = '{"season":1,"episode":1}', keep_up_to_date = 1 WHERE id = ?`, p.ID, itemID); err != nil {
			return err
		}
		for e := 2; e <= 4; e++ {
			it := &mediafiles.Item{TitleID: titleID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: ptr(1), Episode: ptr(e)}, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: verID, State: mediafiles.SlotWanted}
			if e == 2 {
				sl.State, sl.ChosenReleaseID = mediafiles.SlotChosen, &w.relID
			}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots = append(slots, sl.ID)
		}
		window := jsonIDs(slots[:2])
		if _, err := tx.Exec(`UPDATE releases SET name = 'Lab.Show.S01.1080p.WEB.H264-GRP', verdict = json_set(verdict, '$.pick_slot_ids', json(?)), matched_slot_ids = json(?) WHERE id = ?`,
			window, window, w.relID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE downloaders SET writable = 1")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if withFiles {
		w.fake.files[hash] = packFiles()
	}
	return w, m, slots
}

func TestQbitAddStoppedAndFilePriority(t *testing.T) {
	f, u := startFake(t)
	q := NewQbittorrent(u, nil)
	ctx := context.Background()
	if _, err := q.Add(ctx, AddRequest{URL: "magnet:?xt=urn:btih:" + hash, Category: "holyarrs", InfoHash: hash, Stopped: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Add(ctx, AddRequest{File: []byte("d4:infod4:name1:aee"), Category: "holyarrs", InfoHash: hash, Stopped: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Add(ctx, AddRequest{URL: "magnet:?xt=urn:btih:" + hash, Category: "holyarrs", InfoHash: hash}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"true", "true", ""} {
		if f.added[i]["stopped"] != want || f.added[i]["paused"] != want {
			t.Fatalf("add %d: stopped %q paused %q, want %q", i, f.added[i]["stopped"], f.added[i]["paused"], want)
		}
	}
	f.set(hash, map[string]any{"name": "Lab.Show.S01", "category": "holyarrs", "state": "stoppedDL"})
	f.files[hash] = packFiles()
	f.files[hash][2]["priority"] = 0
	j, err := q.Get(ctx, hash)
	if err != nil || len(j.Files) != 5 || j.Files[2].Priority != 0 || j.Files[1].Priority != 1 {
		t.Fatalf("files %+v %v", j.Files, err)
	}
}

func TestGrabPackWindowFetchesOnlyTheWindow(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"add stopped=true", "prio 2|3|4=0", "prio 0|1=1", "start " + hash}
	if !slices.Equal(w.fake.steps, want) {
		t.Fatalf("steps %v, want %v", w.fake.steps, want)
	}
	d := w.download(t, id)
	if !slices.Equal(d.SlotIDs, slots[:2]) || len(d.PackFiles) != 4 {
		t.Fatalf("slots %v pack %+v", d.SlotIDs, d.PackFiles)
	}
	for i, f := range d.PackFiles {
		if f.I != i || f.On != (i < 2) || !slices.Equal(f.Slots, slots[i:i+1]) || !slices.Equal(f.Eps, []int{i + 1}) || len(f.Items) != 1 {
			t.Fatalf("pack file %d: %+v", i, f)
		}
	}
	if got := slotStates(t, w, slots); !slices.Equal(got, []string{"downloading", "downloading", "wanted", "wanted"}) {
		t.Fatalf("slot states %v", got)
	}
	v := DownloadViewOf(d)
	if v.Pack == nil || *v.Pack != (PackRange{From: 1, To: 2, Taken: 2, Total: 4}) {
		t.Fatalf("pack view %+v", v.Pack)
	}
}

func TestGrabPackWindowFailureRemovesAndNeverStarts(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	w.fake.prioFails = true
	if _, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System); err != nil {
		t.Fatal(err)
	}
	for _, s := range w.fake.steps {
		if strings.HasPrefix(s, "start") {
			t.Fatalf("started after an error: %v", w.fake.steps)
		}
	}
	if !slices.Contains(w.fake.deleted, hash+"|true") {
		t.Fatalf("deleted %v", w.fake.deleted)
	}
	var state, rel string
	_ = w.s.Read().QueryRow("SELECT state FROM downloads").Scan(&state)
	_ = w.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", w.relID).Scan(&rel)
	if state != "failed" || rel != "turned_down" {
		t.Fatalf("download %s release %s", state, rel)
	}
	if got := slotStates(t, w, slots[2:]); !slices.Equal(got, []string{"wanted", "wanted"}) {
		t.Fatalf("outside slots %v", got)
	}
}

// Another app (Sonarr, say) already has the same season pack in a shared
// qBittorrent under its own category. The grab follows it as a whole download
// and never switches its files, starts it or removes it with its data, even
// when choosing files would fail.
func TestGrabPackLeavesAnotherAppsTorrentAlone(t *testing.T) {
	for _, prioFails := range []bool{true, false} {
		w, m, slots := packWindowWorld(t, true)
		w.fake.prioFails = prioFails
		w.fake.set(hash, map[string]any{"name": "Lab.Show.S01.1080p", "category": "tv-sonarr", "state": "downloading", "progress": 0.4,
			"save_path": "/data/downloads/sonarr", "size": 4010})
		id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if len(w.fake.steps) != 0 || len(w.fake.prio) != 0 || len(w.fake.deleted) != 0 {
			t.Fatalf("prioFails %v: another app's torrent was changed: steps %v deleted %v", prioFails, w.fake.steps, w.fake.deleted)
		}
		if d := w.download(t, id); d.PackFiles != nil || d.State == "failed" || d.ExternalID != hash {
			t.Fatalf("prioFails %v: download %s %q pack %+v", prioFails, d.State, d.ExternalID, d.PackFiles)
		}
		if got := slotStates(t, w, slots); !slices.Equal(got, []string{"downloading", "downloading", "wanted", "wanted"}) {
			t.Fatalf("prioFails %v: slot states %v", prioFails, got)
		}
	}
}

// The same with a Release that has no info hash (a private site's .torrent
// link): the hash comes from the file, and another app's torrent of that hash
// is still followed whole and never changed or removed.
func TestGrabPackHashFromFileLeavesAnotherAppsTorrentAlone(t *testing.T) {
	tor := []byte("d4:infod6:lengthi3e4:name5:pack.12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee")
	h, err := torrentFileHash(tor)
	if err != nil {
		t.Fatal(err)
	}
	w, m, _ := packWindowWorld(t, false)
	src := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { _, _ = rw.Write(tor) }))
	t.Cleanup(src.Close)
	w.exec(t, "UPDATE releases SET info_hash = NULL, link_ref = ?", src.URL+"/dl/site/?path=x")
	w.exec(t, "UPDATE sources SET allow_local_network = 1, url = ?", src.URL+"/api")
	w.fake.prioFails = true
	w.fake.files[h] = packFiles()
	w.fake.set(h, map[string]any{"name": "Lab.Show.S01.1080p", "category": "tv-sonarr", "state": "downloading", "progress": 0.4,
		"save_path": "/data/downloads/sonarr", "size": 4010})
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.fake.steps) != 0 || len(w.fake.deleted) != 0 {
		t.Fatalf("another app's torrent was changed: steps %v deleted %v", w.fake.steps, w.fake.deleted)
	}
	if d := w.download(t, id); d.PackFiles != nil || d.State == "failed" || d.ExternalID != h {
		t.Fatalf("download %s %q pack %+v", d.State, d.ExternalID, d.PackFiles)
	}
}

// A grab killed after qBittorrent took the pack but before the Download row
// was written finds it again in HolyArrs's own category, still stopped, and
// goes on choosing its files.
func TestGrabPackRetryTakesItsOwnStoppedTorrent(t *testing.T) {
	w, m, _ := packWindowWorld(t, true)
	w.fake.set(hash, map[string]any{"name": "Lab.Show.S01.1080p", "category": "holyarrs", "state": "stoppedDL", "save_path": "/data/downloads/complete", "size": 4010})
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"prio 2|3|4=0", "prio 0|1=1", "start " + hash}; !slices.Equal(w.fake.steps, want) {
		t.Fatalf("steps %v, want %v", w.fake.steps, want)
	}
	if d := w.download(t, id); len(d.PackFiles) != 4 {
		t.Fatalf("pack %+v", d.PackFiles)
	}
}

func TestGrabPackWindowMagnetWaitsForFiles(t *testing.T) {
	w, m, _ := packWindowWorld(t, false)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.PackFiles == nil || len(d.PackFiles) != 0 || len(w.fake.prio) != 0 {
		t.Fatalf("before the file list: pack %+v prio %v", d.PackFiles, w.fake.prio)
	}
	w.fake.set(hash, map[string]any{"name": "Lab.Show.S01.1080p", "category": "holyarrs", "state": "stoppedDL", "save_path": "/data/downloads/complete", "size": 4010})
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	w.fake.files[hash] = packFiles()
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"add stopped=true", "prio 2|3|4=0", "prio 0|1=1", "start " + hash}
	if !slices.Equal(w.fake.steps, want) {
		t.Fatalf("steps %v", w.fake.steps)
	}
	if d := w.download(t, id); len(d.PackFiles) != 4 || !d.PackFiles[1].On || d.PackFiles[2].On {
		t.Fatalf("pack %+v", d.PackFiles)
	}
}

// finishWindow marks the pack finished in the client and its window imported.
func finishWindow(t *testing.T, w *world, id int64, slots []int64) {
	t.Helper()
	w.fake.set(hash, map[string]any{"name": "Lab.Show.S01.1080p", "category": "holyarrs", "state": "stalledUP", "progress": 1, "save_path": "/data/downloads/complete", "size": 2000})
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE downloads SET state = 'finished' WHERE id = ?", id); err != nil {
			return err
		}
		for _, s := range slots {
			if _, err := tx.Exec("UPDATE slots SET state = 'in_library', active_download_id = NULL WHERE id = ?", s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWidenSwitchesOnTheNextEpisodes(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	w.fake.steps = nil
	n, err := m.Widen(ctx, id, 1, domain.System)
	if err != nil || n != 1 {
		t.Fatalf("widen %d %v", n, err)
	}
	if !slices.Equal(w.fake.steps, []string{"prio 2=1"}) || len(w.fake.added) != 1 {
		t.Fatalf("steps %v added %d", w.fake.steps, len(w.fake.added))
	}
	d := w.download(t, id)
	if d.State != "downloading" || !slices.Equal(d.SlotIDs, slots[:3]) || !d.PackFiles[2].On || d.PackFiles[3].On {
		t.Fatalf("download %s slots %v pack %+v", d.State, d.SlotIDs, d.PackFiles)
	}
	var active int64
	_ = w.s.Read().QueryRow("SELECT active_download_id FROM slots WHERE id = ?", slots[2]).Scan(&active)
	if got := slotStates(t, w, slots[2:]); !slices.Equal(got, []string{"downloading", "wanted"}) || active != id {
		t.Fatalf("slot states %v active %d", got, active)
	}
	polled, err := database.Select[domain.Download](ctx, w.s.Read(), "downloads", "state IN ("+activeStates+")")
	if err != nil || len(polled) != 1 {
		t.Fatalf("the poll does not see it again: %v %v", polled, err)
	}
	// The end of the pack: one more, then nothing.
	if n, err := m.Widen(ctx, id, 5, domain.System); err != nil || n != 1 {
		t.Fatalf("widen to the end %d %v", n, err)
	}
	if n, err := m.Widen(ctx, id, 2, domain.System); err != nil || n != 0 {
		t.Fatalf("widen past the end %d %v", n, err)
	}
}

func TestWidenSkipsEpisodesAlreadyThere(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, append(slices.Clone(slots[:2]), slots[2]))
	w.fake.steps = nil
	if n, err := m.Widen(ctx, id, 1, domain.System); err != nil || n != 1 || !slices.Equal(w.fake.steps, []string{"prio 3=1"}) {
		t.Fatalf("widen %d %v steps %v", n, err, w.fake.steps)
	}
}

func TestWidenTorrentGoneClearsPack(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	w.fake.drop(hash)
	if _, err := m.Widen(ctx, id, 2, domain.System); !errors.Is(err, ErrPackGone) {
		t.Fatalf("widen on a gone torrent: %v", err)
	}
	if d := w.download(t, id); d.PackFiles != nil {
		t.Fatalf("pack files kept: %+v", d.PackFiles)
	}
	if got := slotStates(t, w, slots[2:]); !slices.Equal(got, []string{"wanted", "wanted"}) {
		t.Fatalf("slot states %v", got)
	}
}

func TestAdvanceKeepsTheWindowAhead(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	d := w.download(t, id)
	var titleID int64
	_ = w.s.Read().QueryRow("SELECT title_id FROM items WHERE id = ?", d.PackFiles[0].Items[0]).Scan(&titleID)
	// Nothing watched: two ahead already.
	if err := m.Advance(ctx, titleID, map[int64]bool{}, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(w.download(t, id).SlotIDs) != 2 {
		t.Fatal("moved with nothing watched")
	}
	w.fake.steps = nil
	if err := m.Advance(ctx, titleID, map[int64]bool{d.PackFiles[0].Items[0]: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := w.download(t, id); !slices.Equal(got.SlotIDs, slots[:3]) || !slices.Equal(w.fake.steps, []string{"prio 2=1"}) {
		t.Fatalf("slots %v steps %v", got.SlotIDs, w.fake.steps)
	}
}

func TestPackNextFindsTheSeasonPack(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	var titleID int64
	_ = w.s.Read().QueryRow("SELECT title_id FROM items WHERE id = ?", w.download(t, id).PackFiles[0].Items[0]).Scan(&titleID)
	if _, _, err := m.PackNext(ctx, titleID, 2, domain.System); !errors.Is(err, ErrNoPack) {
		t.Fatalf("season 2: %v", err)
	}
	got, n, err := m.PackNext(ctx, titleID, 1, domain.System)
	if err != nil || got != id || n != 2 {
		t.Fatalf("pack next %d %d %v", got, n, err)
	}
}

func TestGrabPackWindowWithoutQbitTakesTheWholeSeason(t *testing.T) {
	w, m, _ := packWindowWorld(t, true)
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec("UPDATE downloaders SET writable = 0"); return err }); err != nil {
		t.Fatal(err)
	}
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.PackFiles != nil || !slices.Equal(w.fake.steps, []string{"add stopped="}) || DownloadViewOf(d).Pack != nil {
		t.Fatalf("pack %+v steps %v", d.PackFiles, w.fake.steps)
	}
}

func TestPackTitlesListsShowsWithAPartialPack(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	if got, err := PackTitles(ctx, w.s.Read()); err != nil || len(got) != 0 {
		t.Fatalf("before the grab: %v %v", got, err)
	}
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	var titleID int64
	_ = w.s.Read().QueryRow("SELECT title_id FROM items WHERE id = ?", w.download(t, id).PackFiles[0].Items[0]).Scan(&titleID)
	if got, err := PackTitles(ctx, w.s.Read()); err != nil || !slices.Equal(got, []int64{titleID}) {
		t.Fatalf("after the grab: %v %v", got, err)
	}
}

func TestPackMagnetWithoutFilesFailsAfterTheStallTime(t *testing.T) {
	w, m, _ := packWindowWorld(t, false)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	w.fake.set(hash, map[string]any{"name": "Lab.Show.S01.1080p", "category": "holyarrs", "state": "stoppedDL", "save_path": "/data/downloads/complete", "size": 4010})
	w.fake.files[hash] = []map[string]any{}
	w.exec(t, "UPDATE downloads SET progress_at = ? WHERE id = ?", database.FormatTime(w.now()), id)
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State == "failed" {
		t.Fatalf("failed too soon: %+v", d)
	}
	w.advance(7 * time.Hour)
	if _, err := m.SyncOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if d := w.download(t, id); d.State != "failed" || !slices.Contains(w.fake.deleted, hash+"|true") {
		t.Fatalf("download %s deleted %v", d.State, w.fake.deleted)
	}
}

func TestPollKeepsAWidenThatLandedAfterItsRead(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	w.exec(t, "UPDATE downloads SET state = 'downloading' WHERE id = ?", id)
	w.fake.set(hash, map[string]any{"name": "Lab.Show.S01.1080p", "category": "holyarrs", "state": "downloading", "progress": 0.3, "save_path": "/data/downloads/complete", "size": 2000})
	stale := w.download(t, id)
	if n, err := m.Widen(ctx, id, 1, domain.System); err != nil || n != 1 {
		t.Fatalf("widen %d %v", n, err)
	}
	var dr domain.Downloader
	if err := database.Get(ctx, w.s.Read(), "downloaders", stale.DownloaderID, &dr); err != nil {
		t.Fatal(err)
	}
	m.apply(ctx, dr, stale, Job{ID: hash, State: StateDownloading, Progress: 0.3}, true, m.settings(ctx))
	d := w.download(t, id)
	if !slices.Equal(d.SlotIDs, slots[:3]) || !d.PackFiles[2].On {
		t.Fatalf("slots %v pack %+v", d.SlotIDs, d.PackFiles)
	}
}

func TestWidenAtOnceNeverPicksTheSameEpisode(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	w.fake.steps = nil
	// Hold the first filePrio call open so, without the lock, the second
	// Widen reads the pack before the first records its pick.
	var first sync.Once
	w.fake.prioWait = func() { first.Do(func() { time.Sleep(300 * time.Millisecond) }) }
	var wg sync.WaitGroup
	got := make([]int, 2)
	for k := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := m.Widen(ctx, id, 1, domain.System)
			if err != nil {
				t.Errorf("widen: %v", err)
			}
			got[k] = n
		}()
	}
	wg.Wait()
	if got[0]+got[1] != 2 {
		t.Fatalf("switched on %v", got)
	}
	w.fake.mu.Lock()
	steps := slices.Clone(w.fake.steps)
	w.fake.mu.Unlock()
	if !slices.Equal(steps, []string{"prio 2=1", "prio 3=1"}) {
		t.Fatalf("steps %v", steps)
	}
	if d := w.download(t, id); !slices.Equal(d.SlotIDs, slots) || !d.PackFiles[2].On || !d.PackFiles[3].On {
		t.Fatalf("slots %v pack %+v", d.SlotIDs, d.PackFiles)
	}
}

func TestWidenRefusedByTheDownloaderUndoesTheRecord(t *testing.T) {
	w, m, slots := packWindowWorld(t, true)
	ctx := context.Background()
	id, err := m.Grab(ctx, w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	finishWindow(t, w, id, slots[:2])
	before := w.download(t, id)
	w.fake.mu.Lock()
	w.fake.prioFails = true
	w.fake.mu.Unlock()
	n, err := m.Widen(ctx, id, 1, domain.System)
	if err == nil || n != 0 {
		t.Fatalf("widen %d %v", n, err)
	}
	d := w.download(t, id)
	if d.State != before.State || !slices.Equal(d.SlotIDs, before.SlotIDs) || d.PackFiles[2].On {
		t.Fatalf("not undone (%v): %s %v %+v", err, d.State, d.SlotIDs, d.PackFiles)
	}
	var active sql.NullInt64
	_ = w.s.Read().QueryRow("SELECT active_download_id FROM slots WHERE id = ?", slots[2]).Scan(&active)
	if got := slotStates(t, w, slots[2:]); !slices.Equal(got, []string{"wanted", "wanted"}) || active.Valid {
		t.Fatalf("slot states %v active %v", got, active)
	}
}
