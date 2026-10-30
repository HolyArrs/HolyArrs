package downloader

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// manageFake answers qBittorrent paths with fixed bodies and status codes
// and keeps each call as "path?form" (multipart fields and file names too).
type manageFake struct {
	mu      sync.Mutex
	calls   []string
	answers map[string]string
	codes   map[string]int
}

func (f *manageFake) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		var form string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
			parts := []string{}
			for k, v := range r.MultipartForm.Value {
				parts = append(parts, k+"="+strings.Join(v, ","))
			}
			for _, fh := range r.MultipartForm.File["torrents"] {
				fd, _ := fh.Open()
				b, _ := io.ReadAll(fd)
				fd.Close()
				parts = append(parts, "file="+fh.Filename+":"+string(b[:1]))
			}
			form = strings.Join(sorted(parts), "&")
		} else {
			_ = r.ParseForm()
			form = r.Form.Encode()
		}
		f.mu.Lock()
		f.calls = append(f.calls, path+"?"+form)
		code, body := f.codes[path], f.answers[path]
		f.mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *manageFake) last(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.calls[i], path+"?") {
			return f.calls[i]
		}
	}
	return ""
}

func (f *manageFake) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, path+"?") {
			n++
		}
	}
	return n
}

func sorted(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

// tinyTorrent is a minimal valid .torrent body (its info dictionary hashes).
const tinyTorrent = "d4:infod6:lengthi1e4:name1:a12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"

func TestLinkHash(t *testing.T) {
	for in, want := range map[string]string{
		"magnet:?xt=urn:btih:" + strings.ToUpper(hashA) + "&dn=x": hashA,
		"magnet:?xt=urn:btih:MFQWCYLBMFQWCYLBMFQWCYLBMFQWCYLB":    "6161616161616161616161616161616161616161",
		hashA:                                  hashA,
		"https://example.test/file.torrent":    "",
		"magnet:?dn=no-hash":                   "",
		"magnet:?xt=urn:btih:nothex-nothex-zz": "",
	} {
		if got := linkHash(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// qBittorrent before 5.2 answers the whole add with "Ok." or "Fails.".
func TestQbitAddTorrentsOldServer(t *testing.T) {
	f := &manageFake{answers: map[string]string{"torrents/info": `[]`, "torrents/add": "Ok."}}
	q := NewQbittorrent(f.start(t), nil)
	out, err := q.AddTorrents(context.Background(), AddBatch{
		Links: []string{"magnet:?xt=urn:btih:" + hashA, "", "https://example.test/x.torrent"}, Files: []TorrentUpload{{Name: `C:\dl\a.torrent`, Data: []byte(tinyTorrent)}},
		Category: "linux", SavePath: "/data/linux", Paused: true,
	})
	if err != nil || out.Added != 2 || out.Pending != 1 || out.Failed != 0 || out.Duplicates != 0 {
		t.Fatalf("%+v %v", out, err)
	}
	got := f.last("torrents/add")
	for _, want := range []string{"category=linux", "savepath=/data/linux", "stopped=true", "paused=true", "file=a.torrent:d", "urls=magnet:?xt=urn:btih:" + hashA + "\nhttps://example.test/x.torrent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("add call %q lacks %q", got, want)
		}
	}
	f.answers["torrents/add"] = "Fails."
	out, err = q.AddTorrents(context.Background(), AddBatch{Links: []string{hashA}})
	if err != nil || out.Failed != 1 || out.Added != 0 {
		t.Fatalf("fails: %+v %v", out, err)
	}
}

// qBittorrent 5.2 answers counts, 409 when nothing was added, 415 for a bad file.
func TestQbitAddTorrentsNewServer(t *testing.T) {
	f := &manageFake{answers: map[string]string{"torrents/info": `[]`, "torrents/add": `{"success_count":1,"failure_count":1,"pending_count":1,"added_torrent_ids":["x"]}`}}
	q := NewQbittorrentKey(f.start(t), "k", nil)
	ctx := context.Background()
	out, err := q.AddTorrents(ctx, AddBatch{Links: []string{"magnet:?xt=urn:btih:" + hashA, "https://example.test/a.torrent", "magnet:?xt=urn:btih:" + strings.Repeat("b", 40)}})
	if err != nil || out.Added != 1 || out.Failed != 1 || out.Pending != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	f.codes = map[string]int{"torrents/add": 409}
	f.answers["torrents/add"] = "Conflict"
	out, err = q.AddTorrents(ctx, AddBatch{Links: []string{hashA}})
	if err != nil || out.Failed != 1 {
		t.Fatalf("409: %+v %v", out, err)
	}
	f.codes = map[string]int{"torrents/add": 415}
	if _, err = q.AddTorrents(ctx, AddBatch{Files: []TorrentUpload{{Name: "a.torrent", Data: []byte(tinyTorrent)}}}); err == nil || !strings.Contains(err.Error(), "could not read") {
		t.Fatalf("415: %v", err)
	}
}

// A torrent the client already holds, a repeat in the same add and a file
// that is not a torrent never reach torrents/add.
func TestQbitAddTorrentsDuplicatesAndBadFiles(t *testing.T) {
	f := &manageFake{answers: map[string]string{"torrents/info": `[{"hash":"` + strings.ToUpper(hashA) + `","state":"uploading"}]`, "torrents/add": "Ok."}}
	q := NewQbittorrent(f.start(t), nil)
	ctx := context.Background()
	out, err := q.AddTorrents(ctx, AddBatch{Links: []string{"magnet:?xt=urn:btih:" + hashA, hashA}, Files: []TorrentUpload{{Name: "notes.txt", Data: []byte("hello")}}})
	if err != nil || out.Duplicates != 2 || out.Failed != 1 || len(out.Bad) != 1 || out.Bad[0] != "notes.txt" {
		t.Fatalf("%+v %v", out, err)
	}
	if n := f.count("torrents/add"); n != 0 {
		t.Fatalf("add was called %d times", n)
	}
	if _, err := q.AddTorrents(ctx, AddBatch{Links: []string{" "}}); err != ErrNothingToAdd {
		t.Fatalf("empty: %v", err)
	}
	two := strings.Repeat("c", 40)
	out, err = q.AddTorrents(ctx, AddBatch{Links: []string{two, "magnet:?xt=urn:btih:" + two}})
	if err != nil || out.Added != 1 || out.Duplicates != 1 {
		t.Fatalf("repeat: %+v %v", out, err)
	}
}

func TestQbitBulkActions(t *testing.T) {
	f := &manageFake{}
	q := NewQbittorrent(f.start(t), nil)
	ctx := context.Background()
	b := strings.Repeat("b", 40)
	hs := []string{strings.ToUpper(hashA), b, hashA}
	want := map[string]string{
		BulkStart: "torrents/start?hashes=" + hashA + "%7C" + b, BulkPause: "torrents/stop?hashes=" + hashA + "%7C" + b,
		BulkRecheck: "torrents/recheck?hashes=" + hashA + "%7C" + b, BulkReannounce: "torrents/reannounce?hashes=" + hashA + "%7C" + b,
		BulkTop: "torrents/topPrio?hashes=" + hashA + "%7C" + b, BulkBottom: "torrents/bottomPrio?hashes=" + hashA + "%7C" + b,
		BulkUp: "torrents/increasePrio?hashes=" + hashA + "%7C" + b, BulkDown: "torrents/decreasePrio?hashes=" + hashA + "%7C" + b,
		BulkForce: "torrents/setForceStart?hashes=" + hashA + "%7C" + b + "&value=true", BulkUnforce: "torrents/setForceStart?hashes=" + hashA + "%7C" + b + "&value=false",
	}
	for _, a := range BulkActions {
		if err := q.Bulk(ctx, a, hs); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		path := strings.SplitN(want[a], "?", 2)[0]
		if got := f.last(path); got != want[a] {
			t.Fatalf("%s: %q, want %q", a, got, want[a])
		}
	}
	if err := q.RemoveMany(ctx, []string{hashA}, true); err != nil || f.last("torrents/delete") != "torrents/delete?deleteFiles=true&hashes="+hashA {
		t.Fatalf("remove %v %q", err, f.last("torrents/delete"))
	}
	if err := q.Bulk(ctx, BulkStart, []string{"all"}); !errors.Is(err, errBadHash) {
		t.Fatalf("all: %v", err)
	}
	if err := q.Bulk(ctx, "explode", hs); !errors.Is(err, ErrBulkAction) {
		t.Fatalf("unknown: %v", err)
	}
	if err := q.Bulk(ctx, BulkStart, nil); err == nil {
		t.Fatal("empty selection passed")
	}
	f.codes = map[string]int{"torrents/bottomPrio": 409}
	if err := q.Bulk(ctx, BulkBottom, hs); !errors.Is(err, ErrQueueOff) {
		t.Fatalf("queue off: %v", err)
	}
	if n := f.count("torrents/bottomPrio"); n != 2 {
		t.Fatalf("a queue move was sent %d times", n)
	}
}
