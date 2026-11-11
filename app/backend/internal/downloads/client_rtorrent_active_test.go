package downloads

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ListActive reads rTorrent's incomplete view and the asked hashes, never the whole list.
func TestRTorrentListActiveReadsIncompleteAndHashes(t *testing.T) {
	const done, gone = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	f := &fakeRTorrent{items: map[string]*rtItem{
		strings.ToUpper(rtHash): {name: "Moving", label: "holyarrs", size: 100, done: 10, state: 1, open: 1, active: 1, dir: "/d"},
		done:                    {name: "Done", label: "holyarrs", size: 100, done: 100, complete: 1, state: 1, open: 1, active: 1, dir: "/d"},
		"CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC": {name: "Seeding", label: "holyarrs", size: 100, done: 100, complete: 1, dir: "/d"},
	}}
	r := NewRTorrent(startRTorrent(t, f), "", "", nil)
	jobs, err := r.ListActive(context.Background(), []string{strings.ToLower(done), strings.ToLower(gone), rtHash})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, j := range jobs {
		ids = append(ids, j.ID)
	}
	slices.Sort(ids)
	if want := []string{rtHash, strings.ToLower(done)}; !slices.Equal(ids, want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	if !slices.Equal(f.views, []string{"incomplete"}) || f.called("system.multicall") != 1 {
		t.Fatalf("views %v, multicalls %d", f.views, f.called("system.multicall"))
	}
}

// The reader reads the whole rTorrent list first, then only what is still downloading, plus the
// Downloads HolyArrs follows. A finished one is seen as finished, a removed one goes missing,
// seeding ones stay listed, and the whole list is read again on the sweep or while the page is open.
func TestSync_RTorrentBackgroundReadsOnlyActive(t *testing.T) {
	const seed, other = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"
	f := &fakeRTorrent{items: map[string]*rtItem{
		strings.ToUpper(ownHash): {name: "Own", label: "holyarrs", size: 100, done: 10, state: 1, open: 1, active: 1, dir: "/d"},
		seed:                     {name: "Seed", label: "holyarrs", size: 100, done: 100, complete: 1, state: 1, open: 1, active: 1, dir: "/d"},
		other:                    {name: "Other", label: "else", size: 100, done: 5, state: 1, open: 1, active: 1, dir: "/d"},
	}}
	rt := NewRTorrent(startRTorrent(t, f), "", "", nil)
	r := newRig(t)
	if err := r.db.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET adapter = 'rtorrent' WHERE id = ?", r.dlID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r.m.Engine = func(domain.Downloader) (Engine, error) { return rt, nil }
	now := time.Unix(1_800_000_000, 0)
	r.s.Now = func() time.Time { return now }
	r.s.Store.Now = r.s.Now
	ctx := context.Background()
	var d domain.Downloader
	if err := database.Get(ctx, r.db.Read(), "downloaders", r.dlID, &d); err != nil {
		t.Fatal(err)
	}
	read := func(want ...string) map[string]Job {
		t.Helper()
		f.mu.Lock()
		f.views = nil
		f.mu.Unlock()
		if err := r.s.read(ctx, d); err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if !slices.Equal(f.views, want) {
			t.Fatalf("views %v, want %v", f.views, want)
		}
		return r.s.Store.Current(d.ID).Items
	}
	items := read("main")
	if len(items) != 2 || items[ownHash].State != StateDownloading || items[strings.ToLower(seed)].State != StateFinished {
		t.Fatalf("first read %+v", items)
	}
	f.mu.Lock()
	f.items[strings.ToUpper(ownHash)].done, f.items[strings.ToUpper(ownHash)].complete = 100, 1
	f.mu.Unlock()
	items = read("incomplete")
	if items[ownHash].State != StateFinished || items[strings.ToLower(seed)].State != StateFinished || len(items) != 2 {
		t.Fatalf("after finishing %+v", items)
	}
	f.mu.Lock()
	delete(f.items, strings.ToUpper(ownHash))
	f.mu.Unlock()
	items = read("incomplete")
	if _, ok := items[ownHash]; ok || len(items) != 1 {
		t.Fatalf("a removed Download is still listed: %+v", items)
	}
	now = now.Add(rtSweep)
	read("main")
	r.s.Store.Viewed()
	read("main")
}

// One background read of a 3,000-torrent rTorrent with 10 still downloading stays small.
func TestRTorrentBackgroundReadIsSmall(t *testing.T) {
	c, ids := rtBigBox(t)
	ctx := context.Background()
	var cur rtCursors
	if _, err := cur.poll(ctx, 1, 1, c, []string{"holyarrs"}, ids, false, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	jobs, err := cur.poll(ctx, 1, 1, c, []string{"holyarrs"}, ids, false, time.Unix(1, 0))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3000 {
		t.Fatalf("%d jobs, want 3000", len(jobs))
	}
	if b := after.TotalAlloc - before.TotalAlloc; b > 5<<20 {
		t.Fatalf("a background read allocated %d bytes, want under 5 MB", b)
	}
}

func BenchmarkRTorrentBackgroundRead(b *testing.B) {
	c, ids := rtBigBox(b)
	ctx := context.Background()
	var cur rtCursors
	if _, err := cur.poll(ctx, 1, 1, c, []string{"holyarrs"}, ids, false, time.Unix(0, 0)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := cur.poll(ctx, 1, 1, c, []string{"holyarrs"}, ids, false, time.Unix(1, 0)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRTorrentFullRead(b *testing.B) {
	c, _ := rtBigBox(b)
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := c.List(ctx, "holyarrs"); err != nil {
			b.Fatal(err)
		}
	}
}

// rtBigBox serves canned answers of a 3,000-torrent rTorrent, 2,990 of them complete, so
// only the client's own work is measured. It returns the client and the hashes still
// downloading, as the Downloads HolyArrs follows.
func rtBigBox(tb testing.TB) (*RTorrent, []string) {
	tb.Helper()
	f := &fakeRTorrent{items: map[string]*rtItem{}}
	var ids []string
	for i := range 3000 {
		h := fmt.Sprintf("%040X", i+1)
		it := &rtItem{name: fmt.Sprintf("Some.Show.S01E%02d.1080p.WEB.h264-GROUP", i%100), label: "holyarrs", size: 2 << 30, done: 2 << 30,
			complete: 1, state: 1, open: 1, active: 1, dir: "/downloads/complete/holyarrs", message: ""}
		if i < 10 {
			it.done, it.complete = 1<<30, 0
			ids = append(ids, strings.ToLower(h))
		}
		f.items[h] = it
	}
	answer := func(view string) []byte {
		params := []any{"", view}
		for _, x := range rtorrentFields {
			params = append(params, x)
		}
		v, _ := f.dispatch("d.multicall2", params)
		var b bytes.Buffer
		b.WriteString(`<?xml version="1.0"?><methodResponse><params><param>`)
		_ = xmlrpcValue(&b, v)
		b.WriteString(`</param></params></methodResponse>`)
		return b.Bytes()
	}
	main, incomplete := answer("main"), answer("incomplete")
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		switch {
		case bytes.Contains(body, []byte("<string>incomplete</string>")):
			_, _ = w.Write(incomplete)
		case bytes.Contains(body, []byte("<string>main</string>")):
			_, _ = w.Write(main)
		default:
			http.Error(w, "unexpected call", http.StatusBadRequest)
		}
	}))
	tb.Cleanup(s.Close)
	return NewRTorrent(s.URL, "", "", nil), ids
}

// notActive claims ListActive but cannot do it, like a guard around an engine
// without it: the background read falls back to a whole read.
type notActive struct{ *fakeClient }

func (notActive) ListActive(context.Context, []string) ([]Job, error) { return nil, ErrNotSupported }

func TestRTorrentPollFallsBackToAWholeReadWhenActiveReadIsNotSupported(t *testing.T) {
	ctx := context.Background()
	f := newFakeClient(Job{ID: "a", Category: "holyarrs", State: StateDownloading})
	e := notActive{f}
	var cur rtCursors
	if _, err := cur.poll(ctx, 1, 1, e, []string{"holyarrs"}, nil, false, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	jobs, err := cur.poll(ctx, 1, 1, e, []string{"holyarrs"}, nil, false, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("background read failed instead of reading the whole list: %v", err)
	}
	if len(jobs) != 1 || f.lists != 2 {
		t.Fatalf("jobs %v, whole reads %d", jobs, f.lists)
	}
}
