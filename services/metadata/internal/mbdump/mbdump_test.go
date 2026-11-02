package mbdump

import (
	"archive/tar"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// row makes one dump row of the table's width: id, gid, name, then \N.
func row(tbl string, id int, name string) string {
	n := len(Tables[tbl].cols)
	f := []string{fmt.Sprint(id), fmt.Sprintf("gid-%d", id), name}
	for len(f) < n {
		f = append(f, `\N`)
	}
	return strings.Join(f[:n], "\t") + "\n"
}

type entry struct{ name, body string }

func tarOf(t *testing.T, ents ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, e := range ents {
		if err := w.WriteHeader(&tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	w.Close()
	return buf.Bytes()
}

func dump(t *testing.T, schema string, artists int) []byte {
	var a strings.Builder
	for i := 1; i <= artists; i++ {
		a.WriteString(row("artist", i, fmt.Sprintf("Artist\\t%d", i)))
	}
	return tarOf(t,
		entry{"COPYING", "x"},
		entry{"SCHEMA_SEQUENCE", schema + "\n"},
		entry{"mbdump/artist", a.String()},
		entry{"mbdump/edit", "1\t2\t3\n"},
		entry{"mbdump/vote", "1\t2\n"},
		entry{"mbdump/release_group", row("release_group", 7, "Blonde")},
	)
}

func build(t *testing.T, dir string, raw []byte, g *Guard) (Result, error) {
	t.Helper()
	b, err := NewBuilder(Options{Dir: dir, DumpDate: "20261028-002228", Guard: g})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Read(context.Background(), bytes.NewReader(raw)); err != nil {
		b.Finish(context.Background())
		return Result{}, err
	}
	return b.Finish(context.Background())
}

func TestFields(t *testing.T) {
	f := Fields("1\t\\N\ta\\tb\\\\c\\nd\t")
	if len(f) != 4 || f[0].S != "1" || !f[1].Null || f[2].S != "a\tb\\c\nd" || f[3].S != "" || f[3].Null {
		t.Fatalf("%+v", f)
	}
}

func TestImportKeepsNeededTablesOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "copy")
	res, err := build(t, dir, dump(t, "31", 3), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows["artist"] != 3 || res.Rows["release_group"] != 1 || len(res.Rows) != 2 {
		t.Fatalf("rows %v", res.Rows)
	}
	db, _ := sql.Open("sqlite", filepath.Join(dir, DBName))
	defer db.Close()
	var n int
	if db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('edit','vote')`).Scan(&n); n != 0 {
		t.Fatal("edit history was kept")
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM artist WHERE gid='gid-2'`).Scan(&name); err != nil || name != "Artist\t2" {
		t.Fatalf("artist %q %v", name, err)
	}
	var date string
	var seq int
	var rows int64
	if err := db.QueryRow(`SELECT dump_date, schema_sequence, rows FROM dump_meta WHERE tbl='artist'`).Scan(&date, &seq, &rows); err != nil ||
		date != "20261028-002228" || seq != 31 || rows != 3 {
		t.Fatalf("dump tag %s %d %d %v", date, seq, rows, err)
	}
	if _, err := Verify(context.Background(), dir, []string{"artist", "release_group"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), dir, []string{"track"}); err == nil {
		t.Fatal("a missing table passed")
	}
}

func TestSchemaMismatchAborts(t *testing.T) {
	_, err := build(t, filepath.Join(t.TempDir(), "c"), dump(t, "30", 1), nil)
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("got %v", err)
	}
	raw := tarOf(t, entry{"mbdump/artist", row("artist", 1, "a")})
	if _, err := build(t, filepath.Join(t.TempDir(), "c"), raw, nil); !errors.Is(err, ErrSchema) {
		t.Fatalf("table before SCHEMA_SEQUENCE got %v", err)
	}
}

func TestWrongWidthFailsTheBuild(t *testing.T) {
	raw := tarOf(t, entry{"SCHEMA_SEQUENCE", "31"}, entry{"mbdump/artist", "1\t2\t3\n"})
	if _, err := build(t, filepath.Join(t.TempDir(), "c"), raw, nil); err == nil || !strings.Contains(err.Error(), "artist has 3 columns") {
		t.Fatalf("got %v", err)
	}
}

func TestGuardCountsPictureBytes(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "f"), make([]byte, 1000), 0o644)
	g := &Guard{Root: root, Limit: 1500, Pictures: func(context.Context) (int64, error) { return 400, nil }}
	if used, err := g.Check(context.Background()); err != nil || used != 1400 {
		t.Fatalf("%d %v", used, err)
	}
	g.Pictures = func(context.Context) (int64, error) { return 600, nil }
	if _, err := (&Guard{Root: root, Limit: 1500, BuildLimit: 1700, Pictures: g.Pictures}).CheckBuild(context.Background()); err != nil {
		t.Fatalf("build cap: %v", err)
	}
	if _, err := g.Check(context.Background()); !errors.Is(err, ErrOverCap) {
		t.Fatalf("pictures past the cap got %v", err)
	}
	// The build stops at its own cap too.
	g.BuildLimit = 1500
	if _, err := build(t, filepath.Join(root, "c"), dump(t, "31", 2), g); !errors.Is(err, ErrOverCap) {
		t.Fatalf("build got %v", err)
	}
}

func TestDecompressPipe(t *testing.T) {
	tool := "lbzip2"
	if _, err := exec.LookPath(tool); err != nil {
		tool = "bzip2"
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no bzip2 tool")
		}
	}
	raw := dump(t, "31", 50)
	cmd := exec.Command(tool, "-c")
	cmd.Stdin = bytes.NewReader(raw)
	packed, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBuilder(Options{Dir: filepath.Join(t.TempDir(), "c"), DumpDate: "d"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := Decompress(context.Background(), tool, bytes.NewReader(packed))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Read(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := b.Finish(context.Background())
	if err != nil || res.Rows["artist"] != 50 {
		t.Fatalf("%v %v", res.Rows, err)
	}
}

// TestRefreshBuildsBesideAndSwaps: a fake dump index moves to a new date;
// the refresher builds the new copy beside the one in use, checks it and
// swaps the pointer while readers keep opening the copy in use, with no
// failed read. The old copy goes after Keep.
func TestRefreshBuildsBesideAndSwaps(t *testing.T) {
	var latest atomic.Value
	latest.Store("20261021-001500")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/LATEST" {
			fmt.Fprintln(w, latest.Load().(string))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	root := t.TempDir()
	sizes := map[string]int{"20261021-001500": 2, "20261028-002228": 5}
	now := time.Unix(10_000, 0)
	r := &Refresher{Root: root, Index: HTTPIndex{Base: srv.URL}, Want: []string{"artist"},
		Now: func() time.Time { return now },
		Build: func(ctx context.Context, date, dir string) (Result, error) {
			b, err := NewBuilder(Options{Dir: dir, DumpDate: date})
			if err != nil {
				return Result{}, err
			}
			if err := b.Read(ctx, bytes.NewReader(dump(t, "31", sizes[date]))); err != nil {
				return Result{}, err
			}
			return b.Finish(ctx)
		}}
	ctx := context.Background()
	if d, swapped, err := r.Once(ctx); err != nil || !swapped || d != "20261021-001500" {
		t.Fatalf("first %s %v %v", d, swapped, err)
	}
	if _, swapped, err := r.Once(ctx); err != nil || swapped {
		t.Fatalf("same dump rebuilt: %v %v", swapped, err)
	}

	var fails, reads atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if n, err := readArtists(root); err != nil || (n != 2 && n != 5) {
					fails.Add(1)
				}
				reads.Add(1)
			}
		}()
	}
	latest.Store("20261028-002228")
	d, swapped, err := r.Once(ctx)
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	if err != nil || !swapped || d != "20261028-002228" {
		t.Fatalf("refresh %s %v %v", d, swapped, err)
	}
	if fails.Load() != 0 || reads.Load() == 0 {
		t.Fatalf("%d of %d reads failed during the swap", fails.Load(), reads.Load())
	}
	if n, err := readArtists(root); err != nil || n != 5 {
		t.Fatalf("after swap %d %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(root, "20261021-001500")); err != nil {
		t.Fatal("old copy dropped before Keep")
	}
	r.Sweep()
	if _, err := os.Stat(filepath.Join(root, "20261021-001500")); err != nil {
		t.Fatal("old copy dropped before Keep")
	}
	now = now.Add(61 * time.Minute)
	if err := r.Sweep(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "20261021-001500")); !os.IsNotExist(err) {
		t.Fatalf("old copy kept after Keep: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "20261028-002228", DBName)); err != nil {
		t.Fatal("copy in use was dropped")
	}
}

func TestRefreshNoSwapLeavesPointer(t *testing.T) {
	root := t.TempDir()
	r := &Refresher{Root: root, Index: fixedIndex("20261028-002228"), Want: []string{"artist"}, NoSwap: true,
		Build: func(ctx context.Context, date, dir string) (Result, error) {
			b, _ := NewBuilder(Options{Dir: dir, DumpDate: date})
			b.Read(ctx, bytes.NewReader(dump(t, "31", 1)))
			return b.Finish(ctx)
		}}
	if _, swapped, err := r.Once(context.Background()); err != nil || swapped {
		t.Fatalf("%v %v", swapped, err)
	}
	if cur, _ := Current(root); cur != "" {
		t.Fatalf("pointer moved to %s", cur)
	}
}

type fixedIndex string

func (f fixedIndex) Latest(context.Context) (string, error) { return string(f), nil }

func readArtists(root string) (int, error) {
	var last error
	for try := 0; try < 3; try++ { // Windows can refuse a file mid-rename
		dir, err := Current(root)
		if err != nil || dir == "" {
			last = fmt.Errorf("no copy: %v", err)
			continue
		}
		db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, DBName)+"?mode=ro")
		if err != nil {
			return 0, err
		}
		var n int
		err = db.QueryRow(`SELECT COUNT(*) FROM artist`).Scan(&n)
		db.Close()
		if err == nil {
			return n, nil
		}
		last = err
	}
	return 0, last
}

// TestCountryCodes: iso_3166_1 is two columns in the dump (area, code); a
// wider pin would skip the table.
func TestCountryCodes(t *testing.T) {
	raw := tarOf(t,
		entry{"SCHEMA_SEQUENCE", "31\n"},
		entry{"mbdump/iso_3166_1", "1\tAF\n221\tGB\n"},
	)
	res, err := build(t, t.TempDir(), raw, nil)
	if err != nil || res.Rows["iso_3166_1"] != 2 {
		t.Fatalf("%v %v", res.Rows, err)
	}
}
