package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/unrar"
	"github.com/bodgit/sevenzip"
	"github.com/therootcompany/xz"
)

// Kind names an archive kind, "" when the name is none of them.
type Kind string

const (
	SevenZip Kind = "7z"
	Zip      Kind = "zip"
	Tar      Kind = "tar"
	TarGzip  Kind = "tar.gz"
	TarBzip2 Kind = "tar.bz2"
	TarXz    Kind = "tar.xz"
	Gzip     Kind = "gz"
	Bzip2    Kind = "bz2"
	Xz       Kind = "xz"
)

var sevenPart = regexp.MustCompile(`(?i)\.7z\.(\d{3})$`)

// Of says which kind a file name is and whether it is the file to open
// (for a 7-Zip set in parts, only .7z.001 is).
func Of(name string) (k Kind, first bool) {
	low := strings.ToLower(filepath.Base(name))
	if m := sevenPart.FindStringSubmatch(low); m != nil {
		return SevenZip, m[1] == "001"
	}
	for _, c := range []struct {
		ext  string
		kind Kind
	}{
		{".tar.gz", TarGzip}, {".tgz", TarGzip}, {".tar.bz2", TarBzip2}, {".tbz2", TarBzip2}, {".tbz", TarBzip2},
		{".tar.xz", TarXz}, {".txz", TarXz}, {".tar", Tar}, {".7z", SevenZip}, {".zip", Zip},
		{".gz", Gzip}, {".gzip", Gzip}, {".bz2", Bzip2}, {".xz", Xz},
	} {
		if strings.HasSuffix(low, c.ext) {
			return c.kind, true
		}
	}
	return "", false
}

const maxEntries = 100000

// Extract unpacks the archive at name (of kind k) into dest, an existing
// folder of its own, writing at most limit bytes.
func Extract(ctx context.Context, name string, k Kind, dest string, limit int64) error {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	w := &writer{ctx: ctx, root: root, left: limit}
	switch k {
	case SevenZip:
		return w.sevenZip(name)
	case Zip:
		return w.zip(name)
	case Tar, TarGzip, TarBzip2, TarXz:
		return w.tarFile(name, k)
	case Gzip, Bzip2, Xz:
		return w.single(name, k)
	}
	return fmt.Errorf("%w: %s", unrar.ErrUnrecognised, filepath.Base(name))
}

type writer struct {
	ctx  context.Context
	root *os.Root
	left int64
	n    int
}

// entry checks one entry before anything is written for it.
func (w *writer) entry(raw string, mode fs.FileMode) (string, error) {
	if err := w.ctx.Err(); err != nil {
		return "", err
	}
	if w.n++; w.n > maxEntries {
		return "", fmt.Errorf("%w: more than %d entries", unrar.ErrUnrecognised, maxEntries)
	}
	name, err := unrar.CleanName(raw)
	if err != nil {
		return "", err
	}
	if t := mode.Type(); t != 0 && t != fs.ModeDir {
		return "", fmt.Errorf("%w: %q is a link or a special file", unrar.ErrUnrecognised, raw)
	}
	return name, nil
}

func (w *writer) dir(name string) error { return w.root.MkdirAll(name, 0o777) }

// file writes one file from r; more than what is left is ErrTooBig.
func (w *writer) file(name string, r io.Reader) error {
	if dir := path.Dir(name); dir != "." {
		if err := w.root.MkdirAll(dir, 0o777); err != nil {
			return err
		}
	}
	f, err := w.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	c, cerr := io.Copy(f, io.LimitReader(ctxReader{w.ctx, r}, w.left+1))
	ferr := f.Close()
	w.left -= c
	if w.left < 0 {
		return unrar.ErrTooBig
	}
	if cerr != nil {
		if w.ctx.Err() != nil {
			return w.ctx.Err()
		}
		var re sevenzip.ReadError
		if errors.As(cerr, &re) && re.Encrypted {
			return unrar.ErrEncrypted
		}
		// %w twice: a full disk stays a syscall error the importer can name.
		return fmt.Errorf("%w: %w", unrar.ErrCorrupt, cerr)
	}
	return ferr
}

// headerEncrypted says a 7-Zip file keeps its list of names under a
// password (7z -mhe): the header at the end is coded with AES (method id
// 06 F1 07 01). The reader gives only a read error for that, so the header
// is looked at once opening has failed. It reads at most 1 MB.
func headerEncrypted(name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	start := make([]byte, 32)
	if _, err := io.ReadFull(f, start); err != nil || !bytes.Equal(start[:6], []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}) {
		return false
	}
	off, size := binary.LittleEndian.Uint64(start[12:20]), binary.LittleEndian.Uint64(start[20:28])
	if size == 0 || size > 1<<20 || off > 1<<50 {
		return false
	}
	head := make([]byte, size)
	if _, err := f.ReadAt(head, int64(32+off)); err != nil {
		return false
	}
	return bytes.Contains(head, []byte{0x06, 0xf1, 0x07, 0x01})
}

// encrypted says a 7-Zip error comes from a password the archive needs.
func encrypted(err error) bool {
	var re sevenzip.ReadError
	return errors.As(err, &re) && re.Encrypted
}

func (w *writer) sevenZip(name string) error {
	rc, err := sevenzip.OpenReader(name)
	if err != nil {
		if encrypted(err) || headerEncrypted(name) {
			return unrar.ErrEncrypted
		}
		return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	defer rc.Close()
	for _, f := range rc.File {
		name, err := w.entry(f.Name, f.Mode())
		if err != nil {
			return err
		}
		if f.Mode().IsDir() {
			if err := w.dir(name); err != nil {
				return err
			}
			continue
		}
		r, err := f.Open()
		if err != nil {
			if encrypted(err) {
				return unrar.ErrEncrypted
			}
			return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
		}
		err = w.file(name, r)
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) zip(name string) error {
	zr, err := zip.OpenReader(name)
	if err != nil {
		return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Flags&0x1 != 0 {
			return unrar.ErrEncrypted // Go's zip reader cannot decrypt
		}
		name, err := w.entry(f.Name, f.Mode())
		if err != nil {
			return err
		}
		if f.Mode().IsDir() || strings.HasSuffix(f.Name, "/") {
			if err := w.dir(name); err != nil {
				return err
			}
			continue
		}
		r, err := f.Open()
		if err != nil {
			return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
		}
		err = w.file(name, r)
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// stream opens the compressed stream of kind k over f.
func stream(f io.Reader, k Kind) (io.Reader, error) {
	switch k {
	case TarGzip, Gzip:
		return gzip.NewReader(f)
	case TarBzip2, Bzip2:
		return bzip2.NewReader(f), nil
	case TarXz, Xz:
		return xz.NewReader(f, 0)
	}
	return f, nil
}

func (w *writer) tarFile(name string, k Kind) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := stream(f, k)
	if err != nil {
		return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			if w.ctx.Err() != nil {
				return w.ctx.Err()
			}
			return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			name, err := w.entry(h.Name, fs.ModeDir)
			if err != nil {
				return err
			}
			if err := w.dir(name); err != nil {
				return err
			}
		case tar.TypeReg:
			name, err := w.entry(h.Name, 0)
			if err != nil {
				return err
			}
			if err := w.file(name, tr); err != nil {
				return err
			}
		case tar.TypeXGlobalHeader:
		default:
			return fmt.Errorf("%w: %q is a link or a special file", unrar.ErrUnrecognised, h.Name)
		}
	}
}

// single unpacks one compressed file to its own name without the ending.
func (w *writer) single(name string, k Kind) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := stream(f, k)
	if err != nil {
		return fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	base := filepath.Base(name)
	clean, err := w.entry(strings.TrimSuffix(base, filepath.Ext(base)), 0)
	if err != nil {
		return err
	}
	return w.file(clean, r)
}

// ctxReader stops a read once ctx ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
