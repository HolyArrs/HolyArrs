package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// permanentFiles are the server's lasting data, relative to the data
// directory: picture records and the matches behind them, video IDs and
// verdicts, install credentials, kept lists and history. Everything else
// is cache (text answers, queues, catalog builds) that refills itself; the
// pictures themselves are rebuilt from their stored links
// (refill-from-links).
var permanentFiles = []string{
	"img/images.sqlite", "herovideo.sqlite", "registered.sqlite", "access.sqlite", "wikidata.sqlite",
	"lists.sqlite", "servicetop.sqlite", "trailers.sqlite", "music.sqlite", "newmusic.sqlite",
	"top10.sqlite", "stats.sqlite", "genretiles.sqlite", "ratings.sqlite", "control.json",
}

// backupPermanent writes one compressed snapshot of the permanent data
// (each database copied consistently while the server runs), keeps the
// newest -keep snapshots in -out, and sends the new one off the VM when
// HOLYARRS_BACKUP_URL names a bucket address (an Oracle Object Storage
// pre-authenticated request ending in /o/; never printed).
//
//	metadata backup-permanent [-data-dir /data] [-out /data/backups] [-keep 3]
func backupPermanent(args []string) error {
	fs := flag.NewFlagSet("backup-permanent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("data-dir", "/data", "server data directory")
	out := fs.String("out", "/data/backups", "snapshot directory")
	keep := fs.Int("keep", 3, "snapshots kept")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *keep < 1 {
		return errors.New("usage: backup-permanent [-data-dir DIR] [-out DIR] [-keep N]")
	}
	if err := os.MkdirAll(*out, 0o700); err != nil {
		return err
	}
	name := "permanent-" + time.Now().UTC().Format("20060102T150405") + ".tar.gz"
	path := filepath.Join(*out, name)
	cleanStale(*out, time.Now().Add(-time.Hour))
	n, err := writeSnapshot(context.Background(), *dir, path)
	if err != nil {
		return err
	}
	st, _ := os.Stat(path)
	fmt.Printf("snapshot %s: %d files, %d KB\n", name, n, st.Size()/1024)
	prune(*out, *keep)
	if u := strings.TrimSpace(os.Getenv("HOLYARRS_BACKUP_URL")); u != "" {
		if err := upload(u, path); err != nil {
			return fmt.Errorf("snapshot kept on the VM; off-VM copy failed: %w", err)
		}
		fmt.Println("off-VM copy sent")
	}
	return nil
}

// cleanStale removes what a killed backup left behind: a half-written
// .part snapshot and .snap-* copies older than cutoff.
func cleanStale(out string, cutoff time.Time) {
	for _, pat := range []string{"*.part", ".snap-*"} {
		m, _ := filepath.Glob(filepath.Join(out, pat))
		for _, p := range m {
			if st, err := os.Stat(p); err == nil && st.ModTime().Before(cutoff) {
				_ = os.RemoveAll(p)
			}
		}
	}
}

// writeSnapshot writes the snapshot to path+".part" and renames it to path
// once synced, so a killed run never leaves a broken file at the final name.
func writeSnapshot(ctx context.Context, dir, path string) (int, error) {
	part := path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := fillSnapshot(ctx, f, dir, filepath.Dir(path))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(part, path)
	}
	if err != nil {
		_ = os.Remove(part)
	}
	return n, err
}

// fillSnapshot copies each permanent file into one tar.gz; databases go
// through VACUUM INTO, so each copy is consistent while the server writes.
func fillSnapshot(ctx context.Context, f *os.File, dir, tmpDir string) (int, error) {
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tmp, err := os.MkdirTemp(tmpDir, ".snap-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	n := 0
	for _, rel := range permanentFiles {
		src := filepath.Join(dir, rel)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		file := src
		if strings.HasSuffix(rel, ".sqlite") {
			file = filepath.Join(tmp, strings.ReplaceAll(rel, "/", "_"))
			if err := vacuumInto(ctx, src, file); err != nil {
				return n, fmt.Errorf("%s: %w", rel, err)
			}
		}
		if err := addFile(tw, file, rel); err != nil {
			return n, err
		}
		n++
	}
	if err := tw.Close(); err != nil {
		return n, err
	}
	return n, gz.Close()
}

func vacuumInto(ctx context.Context, src, dst string) error {
	db, err := sql.Open("sqlite", "file:"+src+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}

func addFile(tw *tar.Writer, file, name string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: st.Size(), ModTime: st.ModTime()}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// prune keeps the newest keep snapshots.
func prune(dir string, keep int) {
	m, _ := filepath.Glob(filepath.Join(dir, "permanent-*.tar.gz"))
	sort.Strings(m)
	for i := 0; i < len(m)-keep; i++ {
		_ = os.Remove(m[i])
	}
}

// upload sends a snapshot to a bucket address (PUT address + name). The
// address is a capability and never appears in an error.
func upload(base, path string) error {
	if !strings.HasPrefix(base, "https://") || !strings.HasSuffix(base, "/") {
		return errors.New("HOLYARRS_BACKUP_URL must be an https address ending in /")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, base+filepath.Base(path), f)
	if err != nil {
		return errors.New("bad bucket address")
	}
	req.ContentLength = st.Size()
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return errors.New("bucket unreachable")
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bucket answered %d", resp.StatusCode)
	}
	return nil
}
