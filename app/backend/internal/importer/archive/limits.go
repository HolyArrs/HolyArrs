package archive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/unrar"
	"github.com/bodgit/sevenzip"
	"github.com/stangelandcl/ppmd"
	"github.com/ulikunitz/xz/lzma"
)

// maxWindow caps the memory a 7-Zip archive may ask its decoder for: the
// LZMA and LZMA2 dictionary and the PPMd model. 7-Zip's strongest usual
// setting (-mx9) uses 64 MB; a few-KB archive could otherwise ask for 3 GB
// or more and take the app down. A bigger ask is refused as not understood.
const maxWindow = 256 << 20

// The 7-Zip reader's own LZMA, LZMA2 and PPMd decoders take the sizes the
// archive names; these do the same work through the same public readers
// after checking the size.
func init() {
	sevenzip.RegisterDecompressor([]byte{0x21}, lzma2Reader)
	sevenzip.RegisterDecompressor([]byte{0x03, 0x01, 0x01}, lzmaReader)
	sevenzip.RegisterDecompressor([]byte{0x03, 0x04, 0x01}, ppmdReader)
}

var errReaders = errors.New("7z: need exactly one reader")

func tooBig(what string, n int64) error {
	return fmt.Errorf("%w: the archive asks for a %d MB %s (most allowed: %d MB)", unrar.ErrUnrecognised, n>>20, what, maxWindow>>20)
}

type closer struct {
	io.Reader
	c io.Closer
}

func (c closer) Close() error { return c.c.Close() }

func lzma2Reader(p []byte, _ uint64, rs []io.ReadCloser) (io.ReadCloser, error) {
	if len(rs) != 1 {
		return nil, errReaders
	}
	if len(p) != 1 || p[0] > 40 {
		return nil, fmt.Errorf("%w: LZMA2 properties", unrar.ErrCorrupt)
	}
	dict := (2 | int64(p[0]&1)) << (p[0]/2 + 11)
	if dict > maxWindow {
		return nil, tooBig("dictionary", dict)
	}
	cfg := lzma.Reader2Config{DictCap: int(dict)}
	if err := cfg.Verify(); err != nil {
		return nil, fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	r, err := cfg.NewReader2(rs[0])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	return closer{r, rs[0]}, nil
}

// lzmaReader rebuilds the classic LZMA header (properties, then the size) in
// front of the stream, as the 7-Zip reader does.
func lzmaReader(p []byte, size uint64, rs []io.ReadCloser) (io.ReadCloser, error) {
	if len(rs) != 1 {
		return nil, errReaders
	}
	if len(p) != 5 {
		return nil, fmt.Errorf("%w: LZMA properties", unrar.ErrCorrupt)
	}
	if dict := int64(binary.LittleEndian.Uint32(p[1:])); dict > maxWindow {
		return nil, tooBig("dictionary", dict)
	}
	h := bytes.NewBuffer(append([]byte(nil), p...))
	_ = binary.Write(h, binary.LittleEndian, size)
	r, err := lzma.NewReader(io.MultiReader(h, rs[0]))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	return closer{r, rs[0]}, nil
}

func ppmdReader(p []byte, size uint64, rs []io.ReadCloser) (io.ReadCloser, error) {
	if len(rs) != 1 {
		return nil, errReaders
	}
	if len(p) != 5 {
		return nil, fmt.Errorf("%w: PPMd properties", unrar.ErrCorrupt)
	}
	mem := int64(binary.LittleEndian.Uint32(p[1:]))
	if mem > maxWindow {
		return nil, tooBig("model", mem)
	}
	if size > 1<<50 {
		return nil, fmt.Errorf("%w: PPMd size", unrar.ErrCorrupt)
	}
	r, err := ppmd.NewH7zReader(rs[0], int(p[0]), int(mem), int(size))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", unrar.ErrCorrupt, err)
	}
	return closer{&r, rs[0]}, nil
}
