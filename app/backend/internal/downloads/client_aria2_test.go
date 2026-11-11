package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const a2Other = "1111111111111111111111111111111111111111"

func aria2Harness(t *testing.T) conformance {
	t.Helper()
	f := &fakeAria2{secret: "<aria2-test-secret>"}
	base := startAria2(t, f)
	const name = "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS.mkv"
	byHash := func() *a2Job {
		for _, j := range f.jobs {
			if j.hash == rtHash {
				return j
			}
		}
		t.Fatal("the added torrent is not in the fake")
		return nil
	}
	return conformance{
		name:    "aria2",
		engine:  NewAria2(base, f.secret, nil),
		badKey:  NewAria2(base, "wrong-secret", nil),
		at:      func(base string) Engine { return NewAria2(base, f.secret, nil) },
		secret:  f.secret,
		release: AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash + "&dn=Lab", Name: "Lab.Torrent.Movie.2024.1080p.WEB.H264-HOLYARRS", Category: "movies", InfoHash: rtHash},
		unknown: a2Other,
		appear: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			j := byHash()
			j.name, j.total, j.done = name, 1000, 500
			j.files = []a2FakeFile{{path: j.dir + "/" + name, length: 1000, done: 500}}
		},
		adds: f.addCount,
		states: func(word string) {
			f.mu.Lock()
			defer f.mu.Unlock()
			j := byHash()
			j.status, j.errCode, j.errMsg, j.seeder, j.name, j.total, j.done = "active", "", "", false, name, 1000, 500
			j.files = []a2FakeFile{{path: j.dir + "/" + name, length: 1000, done: 500}}
			switch word {
			case "metadata":
				j.name, j.total, j.done = "", 0, 0
				j.files = []a2FakeFile{{path: "[METADATA]Lab"}}
			case "seeding":
				j.done, j.seeder = 1000, true
			case "waiting", "paused":
				j.status = word
			case "complete":
				j.status, j.done, j.errCode = "complete", 1000, "0"
			case "error":
				j.status, j.errCode, j.errMsg = "error", "13", "File exists, but a control file(*.aria2) does not exist."
			}
		},
		words: map[string]string{
			"downloading": StateDownloading, "metadata": StateDownloading, "seeding": StateFinished, "waiting": StateQueued,
			"paused": StatePaused, "complete": StateFinished, "error": StatePaused,
		},
	}
}

func TestAria2Conformance(t *testing.T) { runConformance(t, aria2Harness(t)) }

func a2Fast(t *testing.T) {
	t.Helper()
	oldC, oldS := aria2Confirm, aria2Settle
	aria2Confirm.tries, aria2Confirm.gap = 3, time.Millisecond
	aria2Settle.tries, aria2Settle.gap = 3, time.Millisecond
	t.Cleanup(func() { aria2Confirm, aria2Settle = oldC, oldS })
}

func a2Source(t *testing.T, b []byte) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(b) }))
	t.Cleanup(s.Close)
	return s.URL + "/a.torrent"
}

// a2PrivateTorrent is a one-file torrent marked private.
func a2PrivateTorrent(name string) []byte {
	info := fmt.Sprintf("d6:lengthi1e4:name%d:%s12:piece lengthi16384e6:pieces20:012345678901234567897:privatei1ee", len(name), name)
	return []byte("d8:announce27:http://127.0.0.1:9/announce4:info" + info + "e")
}

func TestAria2TokenOnlyWhenSet(t *testing.T) {
	open := &fakeAria2{}
	if _, err := NewAria2(startAria2(t, open), "", nil).Health(context.Background()); err != nil {
		t.Errorf("no secret: %v", err)
	}
	locked := &fakeAria2{secret: "s3cret"}
	base := startAria2(t, locked)
	if _, err := NewAria2(base, "", nil).Health(context.Background()); !errors.Is(err, ErrAuth) {
		t.Errorf("missing secret: %v", err)
	}
	if _, err := NewAria2(base, "s3cret", nil).Health(context.Background()); err != nil {
		t.Errorf("right secret: %v", err)
	}
}

func TestAria2AddsTorrentBytes(t *testing.T) {
	payload := rtTorrentFile("Lab.Movie.2024.mkv", []byte("x"))
	want, _ := torrentFileHash(payload)
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	id, err := e.Add(context.Background(), AddRequest{URL: a2Source(t, payload), Category: "holyarrs", AllowLocal: true})
	if err != nil || id != want {
		t.Fatalf("add: %q %v", id, err)
	}
	if len(f.adds) != 1 || f.adds[0].method != "aria2.addTorrent" || f.called("aria2.addUri") != 0 {
		t.Fatalf("adds %+v", f.adds)
	}
	if o := f.adds[0].opts; len(o) != 1 || o["dir"] != "/downloads/holyarrs" {
		t.Errorf("options %v", o)
	}
	j, err := e.Get(context.Background(), id)
	if err != nil || j.Category != "holyarrs" || j.SavePath != "/downloads/holyarrs" || j.ContentPath != "/downloads/holyarrs/Lab.Movie.2024.mkv" ||
		len(j.Files) != 1 || j.Files[0].Name != "Lab.Movie.2024.mkv" {
		t.Errorf("get: %+v %v", j, err)
	}
}

func TestAria2PrivateTorrentDropsExtraTrackers(t *testing.T) {
	payload := a2PrivateTorrent("Private.Movie.2024.mkv")
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	if _, err := e.Add(context.Background(), AddRequest{File: payload, Category: "movies", AllowLocal: true}); err != nil {
		t.Fatal(err)
	}
	o := f.adds[0].opts
	if v, ok := o["bt-tracker"]; !ok || v != "" || len(o) != 2 {
		t.Errorf("options %v", o)
	}
	for _, k := range []string{"allow-overwrite", "check-integrity", "out", "index-out"} {
		if _, ok := o[k]; ok {
			t.Errorf("sent %s", k)
		}
	}
}

func TestAria2MagnetAdd(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	id, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:FG7HX6UHLFSD6LK5VSPZJVIWYM3YNFXN&dn=Lab", Category: "movies", InfoHash: strings.ToUpper(rtHash)})
	if err != nil || id != rtHash {
		t.Fatalf("add: %q %v", id, err)
	}
	if len(f.adds) != 1 || f.adds[0].method != "aria2.addUri" || len(f.adds[0].uris) != 1 || len(f.adds[0].opts) != 1 {
		t.Fatalf("adds %+v", f.adds)
	}
	if j, err := e.Get(context.Background(), id); err != nil || j.State != StateDownloading || j.Detail != "metadata" || j.Name != "Lab" {
		t.Errorf("metadata job: %+v %v", j, err)
	}
	if _, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies", InfoHash: a2Other}); err == nil {
		t.Error("a magnet for another hash was sent")
	}
}

func TestAria2SaveFolder(t *testing.T) {
	for _, c := range []struct {
		save    string
		pathMap []domain.PathMap
		want    string
	}{
		{"/data/my downloads", nil, "/data/my downloads/movies"},
		{"/data/dl/", nil, "/data/dl/movies"},
		{"/data/dl", []domain.PathMap{{Theirs: "/downloads", Ours: "/data/seedbox"}}, "/downloads/movies"},
		{"relative", nil, "/downloads/movies"},
		{"", nil, "/downloads/movies"},
	} {
		f := &fakeAria2{}
		e := NewAria2(startAria2(t, f), "", nil)
		e.PathMap = c.pathMap
		if _, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies", SavePath: c.save}); err != nil {
			t.Fatal(err)
		}
		if got := f.adds[0].opts["dir"]; got != c.want {
			t.Errorf("save %q: dir %v, want %s", c.save, got, c.want)
		}
	}
	f := &fakeAria2{opts: map[string]string{"dir": `D:\aria2\downloads\`}}
	e := NewAria2(startAria2(t, f), "", nil)
	if _, err := e.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"}); err != nil {
		t.Fatal(err)
	}
	if got := f.adds[0].opts["dir"]; got != `D:\aria2\downloads\movies` {
		t.Errorf("windows folder %v", got)
	}
}

func TestAria2ReconcileOnlySendsNothing(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	ctx := context.Background()
	if _, err := e.Add(ctx, AddRequest{InfoHash: rtHash, Category: "movies", ReconcileOnly: true}); !errors.Is(err, ErrUncertain) {
		t.Errorf("unknown: %v", err)
	}
	f.put(&a2Job{status: "active", hash: rtHash, dir: "/downloads/movies", name: "a.mkv", total: 10})
	if id, err := e.Add(ctx, AddRequest{InfoHash: rtHash, Category: "movies", ReconcileOnly: true}); err != nil || id != rtHash {
		t.Errorf("ours: %q %v", id, err)
	}
	if _, err := e.Add(ctx, AddRequest{InfoHash: rtHash, Category: "tv", ReconcileOnly: true}); !errors.Is(err, ErrUncertain) {
		t.Errorf("another folder: %v", err)
	}
	if _, err := e.Add(ctx, AddRequest{InfoHash: rtHash, Category: "movies", ReconcileOnly: true, Skip: map[string]bool{rtHash: true}}); !errors.Is(err, ErrUncertain) {
		t.Errorf("skipped: %v", err)
	}
	if f.addCount() != 0 || f.called("aria2.removeDownloadResult") != 0 {
		t.Errorf("a reconcile sent %d adds", f.addCount())
	}
}

func TestAria2Confirm(t *testing.T) {
	a2Fast(t)
	magnet := AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"}
	ctx := context.Background()

	lost := &fakeAria2{loseAnswer: true, dropAdd: true}
	if _, err := NewAria2(startAria2(t, lost), "", nil).Add(ctx, magnet); !errors.Is(err, ErrUncertain) {
		t.Errorf("lost answer: %v", err)
	}
	unseen := &fakeAria2{dropAdd: true}
	if _, err := NewAria2(startAria2(t, unseen), "", nil).Add(ctx, magnet); !errors.Is(err, ErrUncertain) {
		t.Errorf("never shown: %v", err)
	}
	late := &fakeAria2{hideFor: 2}
	if id, err := NewAria2(startAria2(t, late), "", nil).Add(ctx, magnet); err != nil || id != rtHash || late.addCount() != 1 {
		t.Errorf("late add: %q %v", id, err)
	}
	cut := &fakeAria2{loseAnswer: true}
	if id, err := NewAria2(startAria2(t, cut), "", nil).Add(ctx, magnet); err != nil || id != rtHash || cut.addCount() != 1 {
		t.Errorf("answer cut but added: %q %v", id, err)
	}

	other := &fakeAria2{addHash: a2Other}
	_, err := NewAria2(startAria2(t, other), "", nil).Add(ctx, magnet)
	if err == nil || errors.Is(err, ErrUncertain) || len(other.jobs) != 0 || other.called("aria2.forceRemove") != 1 {
		t.Errorf("another hash: %v, jobs left %d", err, len(other.jobs))
	}
	dup := &fakeAria2{addError: "12"}
	if _, err := NewAria2(startAria2(t, dup), "", nil).Add(ctx, magnet); !errors.Is(err, ErrUncertain) || len(dup.jobs) != 0 {
		t.Errorf("error 12: %v, jobs left %d", err, len(dup.jobs))
	}
	exists := &fakeAria2{addError: "13"}
	_, err = NewAria2(startAria2(t, exists), "", nil).Add(ctx, magnet)
	if err == nil || errors.Is(err, ErrUncertain) || !strings.Contains(err.Error(), "control file") || len(exists.jobs) != 0 {
		t.Errorf("error 13: %v, jobs left %d", err, len(exists.jobs))
	}
}

func TestAria2DuplicateGuard(t *testing.T) {
	a2Fast(t)
	ctx := context.Background()
	magnet := AddRequest{URL: "magnet:?xt=urn:btih:" + rtHash, Category: "movies"}
	for _, c := range []struct {
		name    string
		job     a2Job
		adds    int
		cleared int
		fails   string
	}{
		{"live in our folder", a2Job{status: "paused", dir: "/downloads/movies"}, 0, 0, ""},
		{"live elsewhere", a2Job{status: "active", dir: "/downloads/tv"}, 0, 0, "another folder"},
		{"errored in our folder", a2Job{status: "error", errCode: "13", dir: "/downloads/movies"}, 1, 1, ""},
		{"removed in our folder", a2Job{status: "removed", errCode: "31", dir: "/downloads/movies"}, 1, 1, ""},
		{"finished in our folder", a2Job{status: "complete", errCode: "0", dir: "/downloads/movies"}, 0, 0, ""},
		{"stopped elsewhere", a2Job{status: "complete", errCode: "0", dir: "/downloads/tv"}, 1, 0, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeAria2{}
			e := NewAria2(startAria2(t, f), "", nil)
			j := c.job
			j.hash, j.name, j.total = rtHash, "a.mkv", 10
			f.put(&j)
			id, err := e.Add(ctx, magnet)
			if c.fails != "" {
				if err == nil || !strings.Contains(err.Error(), c.fails) {
					t.Errorf("add: %q %v", id, err)
				}
			} else if err != nil || id != rtHash {
				t.Errorf("add: %q %v", id, err)
			}
			if f.addCount() != c.adds || f.called("aria2.removeDownloadResult") != c.cleared {
				t.Errorf("%d adds, %d cleared; want %d and %d", f.addCount(), f.called("aria2.removeDownloadResult"), c.adds, c.cleared)
			}
			for _, x := range f.jobs {
				if x.status == "error" && x.errCode == "12" {
					t.Error("aria2 saw a second add of a live hash")
				}
			}
		})
	}
}

func TestAria2ForeignFolderNotTaken(t *testing.T) {
	f := &fakeAria2{}
	e := NewAria2(startAria2(t, f), "", nil)
	f.put(&a2Job{status: "active", hash: rtHash, dir: "/downloads/tv-sonarr", name: "a.mkv", total: 10})
	f.put(&a2Job{status: "active", dir: "/downloads/movies", name: "direct.bin", total: 10})
	ctx := context.Background()
	jobs, err := e.List(ctx, "movies")
	if err != nil || len(jobs) != 0 {
		t.Errorf("list movies: %+v %v", jobs, err)
	}
	all, _ := e.List(ctx, "")
	if len(all) != 1 || all[0].ID != rtHash || all[0].Category != "tv-sonarr" {
		t.Errorf("list all: %+v", all)
	}
	e.Category = "movies"
	if err := e.PauseItem(ctx, rtHash); !errors.Is(err, ErrNotFound) {
		t.Errorf("pause another app's torrent: %v", err)
	}
	if err := e.Remove(ctx, rtHash, false); err == nil || !strings.Contains(err.Error(), "outside the configured category") {
		t.Errorf("remove another app's torrent: %v", err)
	}
	if len(f.jobs) != 2 || f.jobs[0].status != "active" {
		t.Error("another app's torrent was touched")
	}
}

func TestAria2Health(t *testing.T) {
	ctx := context.Background()
	f := &fakeAria2{secret: "s"}
	h, err := NewAria2(startAria2(t, f), "s", nil).Health(ctx)
	if err != nil || h.Version != "1.36.0" || h.Note != "" {
		t.Errorf("health: %+v %v", h, err)
	}
	if _, err := NewAria2(startAria2(t, &fakeAria2{secret: "s"}), "x", nil).Health(ctx); !errors.Is(err, ErrAuth) {
		t.Errorf("wrong secret: %v", err)
	}
	if _, err := NewAria2(startAria2(t, &fakeAria2{features: []string{"HTTPS"}}), "", nil).Health(ctx); err == nil || !strings.Contains(err.Error(), "without BitTorrent") {
		t.Errorf("no BitTorrent: %v", err)
	}
	if _, err := NewAria2(startAria2(t, &fakeAria2{version: "1.33.0"}), "", nil).Health(ctx); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Errorf("old: %v", err)
	}
	if _, err := NewAria2(startAria2(t, &fakeAria2{opts: map[string]string{"dir": "downloads"}}), "", nil).Health(ctx); err == nil {
		t.Error("a relative download folder passed")
	}
	if _, err := NewAria2(a2Canned(t, 200, `{"id":"holyarrs","jsonrpc":"2.0","result":[[{}],[{}]]}`, nil), "", nil).Health(ctx); err == nil ||
		!strings.Contains(err.Error(), "not as aria2 JSON-RPC") {
		t.Errorf("odd answer: %v", err)
	}
	noisy := &fakeAria2{opts: map[string]string{"on-download-stop": "/config/script/delete.sh", "bt-tracker": "udp://t.example:1337/announce"}}
	h, err = NewAria2(startAria2(t, noisy), "", nil).Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"no rpc-secret", "runs a script", "tracker list"} {
		if !strings.Contains(h.Note, want) {
			t.Errorf("note %q lacks %q", h.Note, want)
		}
	}
	if strings.Count(h.Note, "; ") != 3 {
		t.Errorf("note %q", h.Note)
	}
}

func TestAria2SizeRefusalIsFinal(t *testing.T) {
	a2Fast(t)
	f := &fakeAria2{tooLarge: true}
	e := NewAria2(startAria2(t, f), "", nil)
	big := rtTorrentFile("Big.mkv", []byte("x"))
	big = append(big[:len(big)-1], []byte(fmt.Sprintf("7:comment%d:%se", 3<<20, strings.Repeat("c", 3<<20)))...)
	_, err := e.Add(context.Background(), AddRequest{File: big, Category: "movies", AllowLocal: true})
	if err == nil || errors.Is(err, ErrUncertain) || !errors.Is(err, ErrTooLarge) || f.cuts != 1 {
		t.Errorf("got %v after %d sends, want a final size refusal", err, f.cuts)
	}
}
