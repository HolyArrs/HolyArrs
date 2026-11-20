package downloads

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func a2Row(gid, status string, mod func(*a2Status)) a2Status {
	s := a2Status{GID: gid, Status: status, InfoHash: rtHash, Dir: "/d/movies", TotalLength: "100", CompletedLength: "50",
		Files: []a2FileRow{{Path: "/d/movies/a.mkv", Length: "100", CompletedLength: "50", Selected: "true"}}}
	s.Bittorrent.Info.Name, s.Bittorrent.Mode = "a.mkv", "single"
	if mod != nil {
		mod(&s)
	}
	return s
}

func TestAria2States(t *testing.T) {
	meta := func(s *a2Status) {
		s.Bittorrent.Info.Name, s.TotalLength, s.CompletedLength = "", "0", "0"
		s.Files = []a2FileRow{{Path: "[METADATA]Lab"}}
	}
	for _, c := range []struct {
		row    a2Status
		state  string
		detail string
	}{
		{a2Row("a", "active", meta), StateDownloading, "metadata"},
		{a2Row("a", "paused", meta), StateDownloading, "metadata"},
		// The torrent file is fully in; the files it names start next, so the
		// download is not done yet.
		{a2Row("a", "complete", func(s *a2Status) {
			s.ErrorCode = "0"
			meta(s)
		}), StateDownloading, "metadata"},
		{a2Row("a", "active", func(s *a2Status) { s.Seeder = "true" }), StateFinished, "seeding"},
		{a2Row("a", "active", func(s *a2Status) { s.CompletedLength = "100" }), StateFinished, "seeding"},
		{a2Row("a", "active", nil), StateDownloading, "downloading"},
		{a2Row("a", "waiting", nil), StateQueued, "waiting"},
		{a2Row("a", "paused", nil), StatePaused, "paused"},
		{a2Row("a", "complete", func(s *a2Status) { s.ErrorCode = "0" }), StateFinished, "complete"},
		{a2Row("a", "error", func(s *a2Status) { s.ErrorCode, s.ErrorMessage = "12", "InfoHash x is already registered." }), StatePaused, "error: the same torrent is already in aria2"},
		{a2Row("a", "error", func(s *a2Status) {
			s.ErrorCode, s.ErrorMessage = "1", "Bad   tracker http://t.example/a?passkey=secret"
		}), StatePaused, "error: Bad tracker http://t.example"},
	} {
		state, detail := a2State(c.row)
		if state != c.state || !strings.HasPrefix(detail, c.detail) || strings.Contains(detail, "secret") {
			t.Errorf("%s %+v: %s %q, want %s %q", c.row.Status, c.row.Files, state, detail, c.state, c.detail)
		}
	}
	if !cantUseFiles(Job{State: StatePaused, Detail: "error: File exists"}) {
		t.Error("an aria2 error does not wait for a late mount")
	}
}

func TestAria2JobFields(t *testing.T) {
	a := NewAria2("http://x", "", nil)
	j := a.job(a2Row("a", "active", func(s *a2Status) {
		s.Connections, s.NumSeeders, s.UploadLength, s.DownloadSpeed, s.UploadSpeed = "2", "5", "25", "10", "3"
	}))
	if j.ID != rtHash || j.Name != "a.mkv" || j.Category != "movies" || j.SavePath != "/d/movies" || j.ContentPath != "/d/movies/a.mkv" ||
		j.Size != 100 || j.Progress != 0.5 || j.Speed != 10 || j.UpSpeed != 3 || j.Seeds != 5 || j.Peers != 0 || j.Uploaded != 25 ||
		j.Ratio != 0.5 || j.ETA != 5 {
		t.Errorf("job %+v", j)
	}
	done := a.job(a2Row("a", "complete", func(s *a2Status) { s.CompletedLength = "100" }))
	if done.Progress != 1 || done.ETA != 0 || done.State != StateFinished {
		t.Errorf("complete %+v", done)
	}
	empty := a.job(a2Row("a", "active", func(s *a2Status) { s.CompletedLength, s.UploadLength = "0", "9" }))
	if empty.Ratio != 0 {
		t.Errorf("ratio with nothing downloaded %v", empty.Ratio)
	}
	meta := a.job(a2Row("a", "active", func(s *a2Status) {
		s.Bittorrent.Info.Name, s.TotalLength = "", "0"
		s.Files = []a2FileRow{{Path: "[METADATA]Lab Movie"}}
	}))
	if meta.Name != "Lab Movie" || meta.ContentPath != "" {
		t.Errorf("metadata %+v", meta)
	}
	// A magnet's metadata job holds the torrent file's bytes only, so its own
	// length must never read as the download being done.
	doneMeta := a.job(a2Row("a", "active", func(s *a2Status) {
		s.Bittorrent.Info.Name, s.TotalLength, s.CompletedLength = "", "4651", "4651"
		s.Files = []a2FileRow{{Path: "[METADATA]Lab Movie", Length: "4651", CompletedLength: "4651", Selected: "true"}}
	}))
	if doneMeta.State != StateDownloading || doneMeta.Progress != 0 || doneMeta.Size != 4651 {
		t.Errorf("finished metadata %+v", doneMeta)
	}
	// aria2 reports a magnet's metadata job complete for a moment before its
	// content job is listed; it still holds no part of the download.
	completeMeta := a.job(a2Row("a", "complete", func(s *a2Status) {
		s.ErrorCode = "0"
		s.Bittorrent.Info.Name, s.TotalLength, s.CompletedLength = "", "4651", "4651"
		s.Files = []a2FileRow{{Path: "[METADATA]Lab Movie", Length: "4651", CompletedLength: "4651", Selected: "true"}}
	}))
	if completeMeta.State != StateDownloading || completeMeta.Progress != 0 {
		t.Errorf("complete metadata %+v", completeMeta)
	}
}

func TestAria2Pick(t *testing.T) {
	dup := func(s *a2Status) { s.ErrorCode = "12" }
	meta := func(s *a2Status) { s.Bittorrent.Info.Name, s.Files = "", []a2FileRow{{Path: "[METADATA]x"}} }
	handed := func(s *a2Status) { s.FollowedBy = []string{"child"}; meta(s) }
	for _, c := range []struct {
		name string
		rows []a2Status
		want string
	}{
		{"removed only", []a2Status{a2Row("a", "removed", nil)}, ""},
		{"duplicate dropped", []a2Status{a2Row("dup", "error", dup), a2Row("a", "paused", nil)}, "a"},
		{"duplicate alone", []a2Status{a2Row("dup", "error", dup)}, "dup"},
		{"hand-off to child", []a2Status{a2Row("parent", "complete", handed), a2Row("child", "active", nil)}, "child"},
		{"child gone", []a2Status{a2Row("parent", "complete", handed)}, ""},
		{"active over waiting", []a2Status{a2Row("w", "waiting", nil), a2Row("a", "active", nil)}, "a"},
		{"paused over stopped", []a2Status{a2Row("c", "complete", nil), a2Row("p", "paused", nil)}, "p"},
		{"complete over error", []a2Status{a2Row("e", "error", nil), a2Row("c", "complete", nil)}, "c"},
		{"real over metadata", []a2Status{a2Row("m", "active", meta), a2Row("r", "active", nil)}, "r"},
		// The metadata job can still be active when the content job it made is
		// waiting its turn; the content job is the torrent.
		{"waiting real over active metadata", []a2Status{a2Row("m", "active", meta), a2Row("r", "waiting", nil)}, "r"},
		{"paused real over active metadata", []a2Status{a2Row("m", "active", meta), a2Row("r", "paused", nil)}, "r"},
	} {
		got, ok := pick(c.rows)
		if c.want == "" && ok || c.want != "" && got.GID != c.want {
			t.Errorf("%s: %q %v, want %q", c.name, got.GID, ok, c.want)
		}
	}
}

func TestAria2ContentPath(t *testing.T) {
	multi := func(s *a2Status) {
		s.Bittorrent.Info.Name, s.Bittorrent.Mode = "Show.S01", "multi"
		s.Files = []a2FileRow{{Path: "/d/movies/Show.S01/e1.mkv", Selected: "true"}, {Path: "/d/movies/Show.S01/Subs/en.srt", Selected: "true"}}
	}
	for _, c := range []struct {
		row  a2Status
		want string
	}{
		{a2Row("a", "active", nil), "/d/movies/a.mkv"},
		{a2Row("a", "active", multi), "/d/movies/Show.S01"},
		{a2Row("a", "active", func(s *a2Status) { s.Files[0].Path = "relative/a.mkv" }), ""},
		{a2Row("a", "active", func(s *a2Status) { s.Files[0].Path = "/elsewhere/a.mkv" }), ""},
		{a2Row("a", "active", func(s *a2Status) { s.Files[0].Path = "/d/movies" }), ""},
		{a2Row("a", "active", func(s *a2Status) { s.Files[0].Path = "/d/movies/../x/a.mkv" }), ""},
		{a2Row("a", "active", func(s *a2Status) { s.Files = nil }), ""},
		{a2Row("a", "active", func(s *a2Status) { multi(s); s.Bittorrent.Info.Name = "" }), ""},
		{a2Row("a", "active", func(s *a2Status) { multi(s); s.Bittorrent.Info.Name = ".." }), ""},
		{a2Row("a", "active", func(s *a2Status) {
			s.Dir, s.Bittorrent.Mode = `D:\dl\movies`, "multi"
			s.Bittorrent.Info.Name = "Show"
		}), `D:\dl\movies\Show`},
	} {
		if got := content(c.row); got != c.want {
			t.Errorf("%+v: %q, want %q", c.row.Files, got, c.want)
		}
	}
}

func TestAria2GetFilesAndHandOff(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	f.put(&a2Job{status: "active", hash: rtHash, dir: "/downloads/tv", name: "Show.S01", mode: "multi", total: 10, done: 4,
		files: []a2FakeFile{{path: "/downloads/tv/Show.S01/e1.mkv", length: 8, done: 4}, {path: "/downloads/tv/Show.S01/Subs/en.srt", length: 2},
			{path: "/downloads/tv/Show.S01/skip.nfo", length: 1, notSelected: true}}})
	parent := f.put(&a2Job{status: "complete", hash: rtHash, dir: "/downloads/tv", files: []a2FakeFile{{path: "[METADATA]Show"}}})
	child := f.jobs[0]
	parent.followedBy, child.following = []string{child.gid}, parent.gid
	ctx := context.Background()
	j, err := e.Get(ctx, rtHash)
	if err != nil || j.ContentPath != "/downloads/tv/Show.S01" || len(j.Files) != 2 || j.Files[0].Name != "Show.S01/e1.mkv" ||
		j.Files[1].Name != "Show.S01/Subs/en.srt" || j.Files[0].Progress != 0.5 {
		t.Fatalf("get: %+v %v", j, err)
	}
	jobs, _ := e.List(ctx, "tv")
	if len(jobs) != 1 || jobs[0].ID != rtHash || jobs[0].Detail != "downloading" {
		t.Errorf("parent and child listed as %+v", jobs)
	}
	f.drop(child.gid)
	if _, err := e.Get(ctx, rtHash); !errors.Is(err, ErrNotFound) {
		t.Errorf("parent whose child is gone: %v", err)
	}
	if _, err := e.Get(ctx, "nothex"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("bad id: %v", err)
	}
}

// A magnet whose metadata is in and whose content job waits its turn must
// read as the content job: 0.1 done, not the metadata job's 100%.
func TestAria2MagnetHandoverReadsContent(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	parent := f.put(&a2Job{status: "active", hash: rtHash, dir: "/downloads/movies",
		files: []a2FakeFile{{path: "[METADATA]Lab", length: 4651, done: 4651}}})
	child := f.put(&a2Job{status: "waiting", hash: rtHash, dir: "/downloads/movies", name: "Lab.mkv", total: 1000, done: 100})
	parent.followedBy, child.following = []string{child.gid}, parent.gid
	jobs, err := e.List(context.Background(), "movies")
	if err != nil || len(jobs) != 1 || jobs[0].Progress != 0.1 || jobs[0].State != StateQueued || jobs[0].Name != "Lab.mkv" {
		t.Fatalf("list: %+v %v", jobs, err)
	}
	j, err := e.Get(context.Background(), rtHash)
	if err != nil || j.Progress != 0.1 || j.State != StateQueued {
		t.Errorf("get: %+v %v", j, err)
	}
}

// A second download added after the first finished: aria2 still lists the
// finished one, and the second's magnet is fetching its torrent file. The
// second must read as downloading with no progress, never as the torrent
// file's own 100%.
func TestAria2SecondDownloadAfterAFinishedOne(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	f.put(&a2Job{status: "complete", errCode: "0", hash: rtHash, dir: "/downloads/movies", name: "First.mkv", total: 10, done: 10})
	f.put(&a2Job{status: "complete", errCode: "0", hash: a2Other, dir: "/downloads/movies",
		total: 4651, done: 4651, files: []a2FakeFile{{path: "[METADATA]Second.mkv", length: 4651, done: 4651}}})
	jobs, err := e.List(context.Background(), "movies")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ID] = j
	}
	if j := byID[rtHash]; j.State != StateFinished || j.Progress != 1 {
		t.Errorf("the finished first download reads %+v", j)
	}
	if j, ok := byID[a2Other]; !ok || j.State != StateDownloading || j.Progress != 0 || j.Detail != "metadata" {
		t.Errorf("the metadata of the second download reads %+v (listed %v)", j, ok)
	}
}

func TestAria2NoHashJobsInvisible(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	f.put(&a2Job{status: "active", dir: "/downloads/movies", name: "direct.bin", total: 10})
	jobs, err := e.List(context.Background(), "")
	if err != nil || len(jobs) != 0 {
		t.Errorf("listed %+v %v", jobs, err)
	}
	e.Category = "movies"
	if err := e.PauseAll(context.Background()); err != nil || f.jobs[0].status != "active" {
		t.Errorf("pause all touched a direct download: %v", err)
	}
}
