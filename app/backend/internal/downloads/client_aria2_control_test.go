package downloads

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func a2Quiet(t *testing.T) *[]string {
	t.Helper()
	var logged []string
	old := logf
	logf = func(format string, a ...any) { logged = append(logged, fmt.Sprintf(format, a...)) }
	t.Cleanup(func() { logf = old })
	return &logged
}

func TestAria2Remove(t *testing.T) {
	a2Fast(t)
	a2Quiet(t)
	ctx := context.Background()
	for _, c := range []struct {
		name string
		jobs []a2Job
	}{
		{"paused leaves no result", []a2Job{{status: "paused", dir: "/downloads/movies", name: "a.mkv", total: 10}}},
		{"active leaves a removed result", []a2Job{{status: "active", dir: "/downloads/movies", name: "a.mkv", total: 10, done: 2}}},
		{"seeding leaves a complete result", []a2Job{{status: "active", seeder: true, dir: "/downloads/movies", name: "a.mkv", total: 10, done: 10}}},
		{"finished", []a2Job{{status: "complete", errCode: "0", dir: "/downloads/movies", name: "a.mkv", total: 10, done: 10}}},
		{"metadata parent and child", []a2Job{
			{gid: "00000000000000c1", status: "complete", errCode: "0", dir: "/downloads/movies", followedBy: []string{"00000000000000c2"}, files: []a2FakeFile{{path: "[METADATA]a"}}},
			{gid: "00000000000000c2", status: "active", following: "00000000000000c1", dir: "/downloads/movies", name: "a.mkv", total: 10}}},
		{"duplicates too", []a2Job{{status: "paused", dir: "/downloads/movies", name: "a.mkv", total: 10},
			{status: "error", errCode: "12", dir: "/downloads/movies", name: "a.mkv", total: 10}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeAria2{}
			e := NewAria2(startAria2(t, f), "", nil)
			e.Category = "movies"
			for i := range c.jobs {
				j := c.jobs[i]
				j.hash = rtHash
				f.put(&j)
			}
			f.put(&a2Job{status: "active", hash: a2Other, dir: "/downloads/movies", name: "b.mkv", total: 10})
			if err := e.Remove(ctx, rtHash, false); err != nil {
				t.Fatalf("remove: %v", err)
			}
			if len(f.jobs) != 1 || f.jobs[0].hash != a2Other {
				t.Errorf("left %d jobs", len(f.jobs))
			}
			if jobs, _ := e.List(ctx, "movies"); len(jobs) != 1 {
				t.Errorf("listed after remove: %+v", jobs)
			}
			if err := e.Remove(ctx, rtHash, false); !errors.Is(err, ErrNotFound) {
				t.Errorf("second remove: %v", err)
			}
		})
	}
}

func TestAria2RemoveAfterRestart(t *testing.T) {
	a2Fast(t)
	a2Quiet(t)
	here := t.TempDir()
	file := filepath.Join(here, "movies", "a.mkv")
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, []byte("x"), 0o644)
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	e.Category, e.PathMap = "movies", []domain.PathMap{{Theirs: "/downloads", Ours: here}}
	e.Keep = func(context.Context) ([]string, error) { return nil, nil }
	f.put(&a2Job{status: "removed", errCode: "31", hash: rtHash, dir: "/downloads/movies", name: "a.mkv", total: 10, done: 10})
	f.restart()
	if err := e.Remove(context.Background(), rtHash, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("remove: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("deleted: %v", err)
	}
}

func TestAria2RemoveWithFiles(t *testing.T) {
	a2Fast(t)
	const lib = "library"
	for _, c := range []struct {
		name    string
		job     a2Job
		control bool // a .aria2 control file sits beside the data
		other   *a2Job
		keep    func(context.Context) ([]string, error)
		gone    bool
	}{
		{"finished", a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10}, false, nil, nil, true},
		{"unfinished with its control file", a2Job{status: "active", name: "a.mkv", total: 10, done: 0}, true, nil, nil, true},
		{"no Library folders known", a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10}, true, nil,
			func(context.Context) ([]string, error) { return nil, errors.New("locked") }, false},
		{"renamed by aria2", a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10,
			files: []a2FakeFile{{path: "/downloads/movies/a.1.mkv", length: 10, done: 10}}}, false, nil, nil, false},
		{"another job's data", a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10}, false,
			&a2Job{status: "complete", hash: a2Other, dir: "/downloads/movies", name: "a.mkv", total: 10, done: 10}, nil, false},
		{"file exists error", a2Job{status: "error", errCode: "13", name: "a.mkv", total: 10}, false, nil, nil, false},
		{"nothing written", a2Job{status: "active", name: "a.mkv", total: 10}, false, nil, nil, false},
		{"the folder itself", a2Job{status: "complete", errCode: "0", name: "movies", total: 10, done: 10,
			files: []a2FakeFile{{path: "/downloads/movies", length: 10, done: 10}}}, false, nil, nil, false},
		{"a Library folder", a2Job{status: "complete", errCode: "0", name: lib, mode: "multi", total: 10, done: 10,
			files: []a2FakeFile{{path: "/downloads/movies/" + lib + "/Film.mkv", length: 10, done: 10}}}, false, nil, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			here := t.TempDir()
			put := func(rel string) string {
				p := filepath.Join(here, filepath.FromSlash(rel))
				_ = os.MkdirAll(filepath.Dir(p), 0o755)
				_ = os.WriteFile(p, []byte("x"), 0o644)
				return p
			}
			data := put("movies/a.mkv")
			renamed := put("movies/a.1.mkv")
			film := put("movies/" + lib + "/Film.mkv")
			control := ""
			if c.control {
				control = put("movies/a.mkv.aria2")
			}
			f := &fakeAria2{}
			e := NewAria2(startAria2(t, f), "", nil)
			e.Category, e.PathMap = "movies", []domain.PathMap{{Theirs: "/downloads", Ours: here}}
			e.Keep = func(context.Context) ([]string, error) { return []string{filepath.Join(here, "movies", lib)}, nil }
			if c.keep != nil {
				e.Keep = c.keep
			}
			j := c.job
			j.hash, j.dir = rtHash, "/downloads/movies"
			f.put(&j)
			if c.other != nil {
				f.put(c.other)
			}
			logged := a2Quiet(t)
			if err := e.Remove(context.Background(), rtHash, true); err != nil {
				t.Fatalf("remove: %v", err)
			}
			_, err := os.Stat(data)
			if gone := errors.Is(err, os.ErrNotExist); gone != c.gone {
				t.Errorf("data gone %v, want %v (log %v)", gone, c.gone, *logged)
			}
			if control != "" {
				if _, err := os.Stat(control); errors.Is(err, os.ErrNotExist) != c.gone {
					t.Errorf("control file gone %v, want %v", errors.Is(err, os.ErrNotExist), c.gone)
				}
			}
			if !c.gone && len(*logged) == 0 {
				t.Error("kept files were not logged")
			}
			for _, p := range []string{renamed, film, filepath.Join(here, "movies")} {
				if _, err := os.Stat(p); err != nil {
					t.Errorf("%s went: %v", p, err)
				}
			}
		})
	}
	t.Run("no Keep at all", func(t *testing.T) {
		here := t.TempDir()
		data := filepath.Join(here, "movies", "a.mkv")
		_ = os.MkdirAll(filepath.Dir(data), 0o755)
		_ = os.WriteFile(data, []byte("x"), 0o644)
		f := &fakeAria2{}
		e := NewAria2(startAria2(t, f), "", nil)
		e.Category, e.PathMap = "movies", []domain.PathMap{{Theirs: "/downloads", Ours: here}}
		f.put(&a2Job{status: "complete", errCode: "0", hash: rtHash, dir: "/downloads/movies", name: "a.mkv", total: 10, done: 10})
		a2Quiet(t)
		if err := e.Remove(context.Background(), rtHash, true); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(data); err != nil {
			t.Errorf("deleted without knowing the Library folders: %v", err)
		}
	})
}

func TestAria2Controls(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	e.Category = "movies"
	ours := f.put(&a2Job{status: "active", hash: rtHash, dir: "/downloads/movies", name: "a.mkv", total: 10})
	theirs := f.put(&a2Job{status: "active", hash: a2Other, dir: "/downloads/tv", name: "b.mkv", total: 10})
	stopped := f.put(&a2Job{status: "error", errCode: "1", hash: "2222222222222222222222222222222222222222", dir: "/downloads/movies", name: "c.mkv", total: 10})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := e.PauseItem(ctx, rtHash); err != nil || ours.status != "paused" {
			t.Errorf("pause %d: %v %s", i, err, ours.status)
		}
	}
	if f.called("aria2.forcePause") != 2 || f.called("aria2.pause") != 0 {
		t.Errorf("pause calls: force %d plain %d", f.called("aria2.forcePause"), f.called("aria2.pause"))
	}
	if st, err := e.Status(ctx); err != nil || !st.Paused || st.Speed != 300 || st.UpSpeed != 40 {
		t.Errorf("status: %+v %v", st, err)
	}
	for i := 0; i < 2; i++ {
		if err := e.ResumeItem(ctx, rtHash); err != nil || ours.status != "active" {
			t.Errorf("resume %d: %v %s", i, err, ours.status)
		}
	}
	if err := e.ResumeItem(ctx, stopped.hash); err == nil || !strings.Contains(err.Error(), "grab it again") {
		t.Errorf("resume a stopped job: %v", err)
	}
	if err := e.PauseAll(ctx); err != nil || ours.status != "paused" || theirs.status != "active" {
		t.Errorf("pause all: %v ours %s theirs %s", err, ours.status, theirs.status)
	}
	if err := e.ResumeAll(ctx); err != nil || ours.status != "active" {
		t.Errorf("resume all: %v %s", err, ours.status)
	}
	if f.called("aria2.pauseAll") != 0 || f.called("aria2.unpauseAll") != 0 || f.called("aria2.forcePauseAll") != 0 {
		t.Error("a whole-aria2 pause was sent")
	}
	if st, err := e.Status(ctx); err != nil || st.Paused {
		t.Errorf("status running: %+v %v", st, err)
	}
	if err := e.MoveTop(ctx, rtHash); !errors.Is(err, ErrNotSupported) {
		t.Errorf("move top: %v", err)
	}
	var _ Controller = e
}

func TestAria2SpeedLimit(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	ctx := context.Background()
	if err := e.SetSpeedLimit(ctx, 1048576, 524288); err != nil {
		t.Fatal(err)
	}
	if f.opts["max-overall-download-limit"] != "1048576" || f.opts["max-overall-upload-limit"] != "524288" {
		t.Errorf("options %v", f.opts)
	}
	if st, err := e.Status(ctx); err != nil || st.SpeedLimit != 1048576 || st.UpLimit != 524288 || st.Paused {
		t.Errorf("status: %+v %v", st, err)
	}
	if err := e.SetSpeedLimit(ctx, 0, -5); err != nil || f.opts["max-overall-upload-limit"] != "0" {
		t.Errorf("no caps: %v %v", err, f.opts)
	}
	f.capsIgnore = true
	if err := e.SetSpeedLimit(ctx, 2048, 0); err == nil {
		t.Error("a cap aria2 did not keep was taken")
	}
}

// A job aria2 took over files already on disk for keeps them on a remove with
// files: only a job aria2 had to write itself counts as HolyArrs's grab.
func TestAria2RemoveKeepsAdoptedFiles(t *testing.T) {
	a2Fast(t)
	for _, c := range []struct {
		name    string
		opts    map[string]string
		fault   bool
		job     a2Job
		control bool
		gone    bool
	}{
		{"checked existing files", map[string]string{"check-integrity": "true"},
			false, a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10}, false, false},
		{"seeding unverified files", map[string]string{"bt-seed-unverified": "true"},
			false, a2Job{status: "active", seeder: true, name: "a.mkv", total: 10, done: 10}, false, false},
		{"checked with a control file", map[string]string{"check-integrity": "true"},
			false, a2Job{status: "active", name: "a.mkv", total: 10, done: 4}, true, false},
		{"options unknown", nil, true, a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10}, false, false},
		{"downloaded by aria2", nil, false, a2Job{status: "complete", errCode: "0", name: "a.mkv", total: 10, done: 10}, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			here := t.TempDir()
			data := filepath.Join(here, "movies", "a.mkv")
			_ = os.MkdirAll(filepath.Dir(data), 0o755)
			_ = os.WriteFile(data, []byte("x"), 0o644)
			if c.control {
				_ = os.WriteFile(data+".aria2", []byte("x"), 0o644)
			}
			f := &fakeAria2{opts: c.opts, optionFault: c.fault}
			e := NewAria2(startAria2(t, f), "", nil)
			e.Category, e.PathMap = "movies", []domain.PathMap{{Theirs: "/downloads", Ours: here}}
			e.Keep = func(context.Context) ([]string, error) { return nil, nil }
			j := c.job
			j.hash, j.dir = rtHash, "/downloads/movies"
			f.put(&j)
			logged := a2Quiet(t)
			if err := e.Remove(context.Background(), rtHash, true); err != nil {
				t.Fatalf("remove: %v", err)
			}
			_, err := os.Stat(data)
			if gone := errors.Is(err, os.ErrNotExist); gone != c.gone {
				t.Errorf("data gone %v, want %v (log %v)", gone, c.gone, *logged)
			}
			if !c.gone && len(*logged) == 0 {
				t.Error("kept files were not logged")
			}
		})
	}
}

// aria2 on Windows reports folders with backslashes; the shared rules read
// them the same way content builds them.
func TestAria2RemoveWithFilesOnWindows(t *testing.T) {
	a2Fast(t)
	here := t.TempDir()
	data := filepath.Join(here, "movies", "a.mkv")
	_ = os.MkdirAll(filepath.Dir(data), 0o755)
	_ = os.WriteFile(data, []byte("x"), 0o644)
	f := &fakeAria2{opts: map[string]string{"dir": `C:\dl`}}
	e := NewAria2(startAria2(t, f), "", nil)
	e.Category, e.PathMap = "movies", []domain.PathMap{{Theirs: `C:\dl`, Ours: here}}
	e.Keep = func(context.Context) ([]string, error) { return nil, nil }
	f.put(&a2Job{status: "complete", errCode: "0", hash: rtHash, dir: `C:\dl\movies`, name: "a.mkv", total: 10, done: 10,
		files: []a2FakeFile{{path: `C:\dl\movies\a.mkv`, length: 10, done: 10}}})
	logged := a2Quiet(t)
	if err := e.Remove(context.Background(), rtHash, true); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("kept: %v (log %v)", err, *logged)
	}
}

// A job is ours only in the exact folder HolyArrs adds into, not in any
// folder that ends in the category's name.
func TestAria2OwnsOnlyItsFolder(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name, save, ours, theirs string
	}{
		{"aria2's folder", "", "/downloads/tv", "/home/u/other/tv"},
		{"aria2's folder with a trailing slash", "", "/downloads/tv/", "/downloads/x/tv"},
		{"the save folder", "/data", "/data/tv", "/downloads/tv"},
		{"another category", "", "/downloads/anime", "/elsewhere/anime"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeAria2{}
			e := NewAria2(startAria2(t, f), "", nil)
			e.Category, e.Others, e.SaveRoot = "tv", []string{"anime"}, c.save
			ours := f.put(&a2Job{status: "active", hash: rtHash, dir: c.ours, name: "a.mkv", total: 10})
			theirs := f.put(&a2Job{status: "active", hash: a2Other, dir: c.theirs, name: "b.mkv", total: 10})
			if err := e.PauseAll(ctx); err != nil || ours.status != "paused" || theirs.status != "active" {
				t.Errorf("pause all: %v ours %s theirs %s", err, ours.status, theirs.status)
			}
			if err := e.PauseItem(ctx, a2Other); !errors.Is(err, ErrNotFound) {
				t.Errorf("pause theirs: %v", err)
			}
			if err := e.Remove(ctx, a2Other, false); err == nil {
				t.Error("removed a job outside the folder")
			}
			if len(f.jobs) != 2 {
				t.Errorf("left %d jobs", len(f.jobs))
			}
		})
	}
}
