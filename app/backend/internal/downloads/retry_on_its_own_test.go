package downloads

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

const (
	notYetThere = "HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again"
	noWrite     = "HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again"
	noMatch     = "no file in the download matched an episode; match the files by hand"
)

var retryNow = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func wi(slot, dl int64, reason string, ago time.Duration) mediafiles.WaitingImport {
	return mediafiles.WaitingImport{SlotID: slot, DownloadID: dl, Reason: reason, Since: retryNow.Add(-ago)}
}

func TestRetryWait(t *testing.T) {
	cases := []struct {
		tries int
		want  time.Duration
	}{
		{0, 5 * time.Minute},
		{1, 15 * time.Minute},
		{2, time.Hour},
		{3, time.Hour},
		{500, time.Hour},
	}
	for _, c := range cases {
		if got := retryWait(c.tries); got != c.want {
			t.Fatalf("retryWait(%d) = %v, want %v", c.tries, got, c.want)
		}
	}
}

func TestRetryDue(t *testing.T) {
	cases := []struct {
		name    string
		waiting []mediafiles.WaitingImport
		marks   map[int64]retryMark
		want    []int64
		left    []int64
	}{
		{"nothing waits", nil, map[int64]retryMark{}, nil, nil},
		{"passing fault for 6 min: first try", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 6*time.Minute)}, map[int64]retryMark{}, []int64{10}, nil},
		{"movie file missing for a minute: first try", []mediafiles.WaitingImport{wi(1, 10, "the file could not be placed: the file is not in the download folder any more", time.Minute)}, map[int64]retryMark{}, []int64{10}, nil},
		{"missing file after many tries still checks every minute", []mediafiles.WaitingImport{wi(1, 10, importer.CopyingWhy, 48*time.Hour)}, map[int64]retryMark{10: {40, retryNow.Add(-time.Minute)}}, []int64{10}, []int64{10}},
		{"failed a minute ago waits", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, time.Minute)}, map[int64]retryMark{}, nil, nil},
		{"a permission waits for a person", []mediafiles.WaitingImport{wi(1, 10, noWrite, time.Hour)}, map[int64]retryMark{}, nil, nil},
		{"a match by hand waits for a person", []mediafiles.WaitingImport{wi(1, 10, noMatch, time.Hour)}, map[int64]retryMark{}, nil, nil},
		{"second try waits 15 min after the first", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, time.Hour)}, map[int64]retryMark{10: {1, retryNow.Add(-10 * time.Minute)}}, nil, []int64{10}},
		{"second try after 15 min", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, time.Hour)}, map[int64]retryMark{10: {1, retryNow.Add(-16 * time.Minute)}}, []int64{10}, []int64{10}},
		{"third try waits an hour", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 2*time.Hour)}, map[int64]retryMark{10: {2, retryNow.Add(-50 * time.Minute)}}, nil, []int64{10}},
		{"after many tries still hourly", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 48*time.Hour)}, map[int64]retryMark{10: {40, retryNow.Add(-61 * time.Minute)}}, []int64{10}, []int64{10}},
		// A failure that leaves updated_at old still waits from the last try.
		{"old updated_at, recent try", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 24*time.Hour)}, map[int64]retryMark{10: {1, retryNow.Add(-time.Minute)}}, nil, []int64{10}},
		{"pack whose episodes all await another import try", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 6*time.Minute), wi(2, 10, importer.SlowReadWhy, 6*time.Minute)}, map[int64]retryMark{}, []int64{10}, nil},
		{"mixed pack still tries its retryable episode", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 6*time.Minute), wi(2, 10, noWrite, time.Minute)}, map[int64]retryMark{}, []int64{10}, nil},
		{"pack with one retryable episode that failed just now", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 6*time.Minute), wi(2, 10, importer.SlowReadWhy, time.Minute)}, map[int64]retryMark{}, nil, nil},
		{"a settled download is forgotten", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 6*time.Minute)}, map[int64]retryMark{11: {3, retryNow}}, []int64{10}, nil},
		{"two downloads in Slot order", []mediafiles.WaitingImport{wi(1, 11, importer.SlowReadWhy, 6*time.Minute), wi(2, 10, importer.SlowReadWhy, 6*time.Minute)}, map[int64]retryMark{}, []int64{11, 10}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := retryDue(c.waiting, c.marks, retryNow)
			if !slices.Equal(got, c.want) {
				t.Fatalf("due %v, want %v", got, c.want)
			}
			var left []int64
			for id := range c.marks {
				left = append(left, id)
			}
			slices.Sort(left)
			if !slices.Equal(left, c.left) {
				t.Fatalf("marks kept %v, want %v", left, c.left)
			}
		})
	}
}

func TestRetryCards(t *testing.T) {
	at := retryNow.Add(4 * time.Minute)
	cases := []struct {
		name    string
		waiting []mediafiles.WaitingImport
		marks   map[int64]retryMark
		want    map[int64]time.Time
	}{
		{"one card: its download's time", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, time.Minute)}, nil, map[int64]time.Time{1: at}},
		{"missing file: checked every minute", []mediafiles.WaitingImport{wi(1, 10, importer.CopyingWhy, time.Minute)}, nil, map[int64]time.Time{1: retryNow}},
		{"a person's card gets none", []mediafiles.WaitingImport{wi(1, 10, noWrite, time.Minute)}, nil, map[int64]time.Time{}},
		// Every passing card of a pack shows the one try of the download, taken from the newest.
		{"pack: one time for the download", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 3*time.Minute), wi(2, 10, importer.SlowReadWhy, time.Minute)}, nil, map[int64]time.Time{1: at, 2: at}},
		{"mixed pack: the passing card only", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, time.Minute), wi(2, 10, noWrite, time.Minute)}, nil, map[int64]time.Time{1: at}},
		{"after a try: from the try and its wait", []mediafiles.WaitingImport{wi(1, 10, importer.SlowReadWhy, 24*time.Hour)}, map[int64]retryMark{10: {2, retryNow}}, map[int64]time.Time{1: retryNow.Add(time.Hour)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := retryCards(c.waiting, retryPlan(c.waiting, c.marks))
			if len(got) != len(c.want) {
				t.Fatalf("cards %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if !got[k].Equal(v) {
					t.Fatalf("cards %v, want %v", got, c.want)
				}
			}
		})
	}
}

// A pack with one episode that needs a person: the retry wakes only the
// episodes whose files may have arrived, and leaves the other card as it is.
func TestRetryWakesOnlyPassingEpisodes(t *testing.T) {
	w, m, id, ids := packWorld(t)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	for i, sid := range ids {
		reason := notYetThere
		if i == len(ids)-1 {
			reason = noWrite
		}
		w.exec(t, "UPDATE slots SET state = 'needs_a_decision', last_reason = ?, active_download_id = ? WHERE id = ?", reason, id, sid)
	}
	if err := m.importAgainWith(context.Background(), id, domain.System, importer.RetriesOnItsOwn); err != nil {
		t.Fatal(err)
	}
	got := slotStates(t, w, ids)
	want := []string{string(mediafiles.SlotImporting), string(mediafiles.SlotImporting), string(mediafiles.SlotNeedsADecision)}
	if !slices.Equal(got, want) {
		t.Fatalf("slots %v, want %v", got, want)
	}
}

// A finished download whose files arrive after the import ran (a sync from a
// seedbox) is imported on its own once they are there, with no click, and
// the files are checked every minute while it waits.
func TestFilesThatArriveLateImportOnTheirOwn(t *testing.T) {
	w, m, e, id, root := unseenJob(t)
	clock := time.Now().UTC()
	m.Now = func() time.Time { return clock }
	ctx := context.Background()

	m.retryWaitingImports(ctx)
	if m.retried[id].tries != 0 {
		t.Fatal("tried before the wait")
	}
	for i, wait := range []time.Duration{time.Minute, time.Minute, time.Minute, time.Minute} {
		clock = clock.Add(wait - 30*time.Second)
		m.retryWaitingImports(ctx)
		if m.retried[id].tries != i {
			t.Fatalf("try %d came early: %d tries", i+1, m.retried[id].tries)
		}
		clock = clock.Add(30 * time.Second)
		m.retryWaitingImports(ctx)
		if m.retried[id].tries != i+1 {
			t.Fatalf("try %d not made: %d tries", i+1, m.retried[id].tries)
		}
		wantUnseenCard(t, w)
	}

	job := filepath.Join(root, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	e.job.SavePath, e.job.ContentPath = root, job
	clock = clock.Add(time.Minute)
	m.retryWaitingImports(ctx)
	if sl := w.slot(t); sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
	if n := w.countDownloads(t); n != 1 {
		t.Fatalf("%d downloads", n)
	}
	m.retryWaitingImports(ctx)
	if _, kept := m.retried[id]; kept {
		t.Fatal("a settled download keeps its tries")
	}
}

// A file missing from an otherwise visible Downloader folder stays linked to
// its finished download and imports itself after a later local copy arrives.
func TestMissingFileInVisibleFolderImportsWhenItArrives(t *testing.T) {
	w, m, e, id, root := unseenJob(t)
	job := filepath.Join(root, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv")
	w.exec(t, "UPDATE downloaders SET protocol = 'torrent', adapter = 'qbittorrent'")
	w.exec(t, `UPDATE downloads SET files_seen = json_array(json_object('name', ?, 'size', 2048)), save_path = ? WHERE id = ?`, filepath.Base(job), root, id)
	e.job.SavePath, e.job.ContentPath = root, root
	w.exec(t, "UPDATE slots SET state = 'importing', active_download_id = ? WHERE id = ?", id, w.slotID)
	m.OnFinished(context.Background(), id)
	if sl := w.slot(t); sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != importer.NotPresentYetWhy {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}

	clock := time.Now().UTC().Add(2 * time.Minute)
	m.Now = func() time.Time { return clock }
	m.retryWaitingImports(context.Background())
	if m.retried[id].tries != 1 || w.countDownloads(t) != 1 {
		t.Fatal("missing file was not retried on its own")
	}
	if err := os.WriteFile(job, make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Minute)
	m.retryWaitingImports(context.Background())
	if sl := w.slot(t); sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
	if n := w.countDownloads(t); n != 1 {
		t.Fatalf("%d downloads", n)
	}
}
