package unrar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/nwaples/rardecode/v2"
)

// ErrTooBig says a set wrote more than the caller allowed; what was written
// is left for the caller to remove.
var ErrTooBig = errors.New("the RAR files unpack to more than was allowed")

// maxDictionary caps the decode window a set may ask for (RAR 5 allows up to
// 4 GB); WinRAR's usual 32 MB and the larger sizes scene groups use fit well
// inside it, and a bigger ask is refused instead of taking the memory.
const maxDictionary = 256 << 20

// ExtractCompressed unpacks a compressed set (RAR 4 or RAR 5, one volume or
// many) into dest with the built-in decoder. dest must be an existing folder
// of its own. Names pass the same checks as Extract, links and special files
// are refused, every file is written through a handle on dest (so no path,
// link or junction leads outside it), existing files are never overwritten,
// and it stops once more than limit bytes were written or ctx ends (a set
// can make the decoder work long while writing little). Checksums are
// checked as each file ends.
func ExtractCompressed(ctx context.Context, first, dest string, limit int64) error {
	rc, err := rardecode.OpenReader(first, rardecode.MaxDictionarySize(maxDictionary))
	if err != nil {
		return decodeErr(err)
	}
	defer rc.Close()
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	var written int64
	for n := 0; ; n++ {
		if n >= maxEntries {
			return fmt.Errorf("%w: more than %d entries", ErrUnrecognised, maxEntries)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := rc.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return decodeErr(err)
		}
		if h.Encrypted || h.HeaderEncrypted {
			return ErrEncrypted
		}
		name, err := cleanName(h.Name)
		if err != nil {
			return err
		}
		if t := h.Mode().Type(); h.LinkType != 0 || (t != 0 && t != fs.ModeDir) {
			return fmt.Errorf("%w: %q is a link or a special file", ErrUnrecognised, h.Name)
		}
		if h.IsDir {
			if err := root.MkdirAll(name, 0o777); err != nil {
				return err
			}
			continue
		}
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o777); err != nil {
				return err
			}
		}
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			return err
		}
		// One byte past what is left shows a set that writes too much.
		c, cerr := io.Copy(f, io.LimitReader(ctxReader{ctx, &rc.Reader}, limit-written+1))
		ferr := f.Close()
		written += c
		if written > limit {
			return ErrTooBig
		}
		if cerr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return decodeErr(cerr)
		}
		if ferr != nil {
			return ferr
		}
	}
}

// decodeErr turns the decoder's errors into this package's kinds.
func decodeErr(err error) error {
	switch {
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted), errors.Is(err, rardecode.ErrBadPassword):
		return ErrEncrypted
	case errors.Is(err, rardecode.ErrDictionaryTooLarge):
		return fmt.Errorf("%w: %v", ErrUnrecognised, err)
	}
	return fmt.Errorf("%w: %v", ErrCorrupt, err)
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
