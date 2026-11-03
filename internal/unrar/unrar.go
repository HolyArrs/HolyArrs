// Package unrar reads RAR files that use "store" (no compression), in both
// the RAR 4 and RAR 5 layouts, including sets split over many volumes. That
// is the common way releases are packed. Compressed or encrypted sets are
// reported (List says so, Extract refuses) so a caller can hand them to a
// system tool instead.
//
// It reads untrusted files: sizes are bounded, every header is checked
// against the volume's length, entry paths that are absolute or climb out
// of the target folder are refused, and reading stops at the first header
// that does not make sense. It only ever reads the volumes it is given.
package unrar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var (
	// ErrCompressed says an entry uses RAR compression, which this package does not unpack.
	ErrCompressed = errors.New("the RAR files use compression")
	// ErrEncrypted says an entry or a header is password protected.
	ErrEncrypted = errors.New("the RAR files are password protected")
	// ErrUnsafePath says an entry would land outside the target folder.
	ErrUnsafePath = errors.New("a RAR entry has an unsafe path")
	// ErrCorrupt says the set is damaged, cut short or has a volume missing.
	ErrCorrupt = errors.New("the RAR files are damaged or incomplete")
)

const (
	maxVolumes   = 2000
	maxEntries   = 100000
	maxNameLen   = 4096
	maxHeader    = 1 << 20
	maxDataBytes = int64(1) << 50
)

// Entry is one file in a set. Directories and links are not listed.
type Entry struct {
	Name       string // cleaned, with "/" between folders
	Size       int64  // bytes once unpacked
	Compressed bool
	Encrypted  bool
}

func corrupt(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, a...))
}

var (
	partStyle = regexp.MustCompile(`(?i)^(.*\.part)(\d{1,6})(\.rar)$`)
	oldStyle  = regexp.MustCompile(`(?i)^(.*\.)([a-z])(\d\d)$`)
)

// IsVolume says a file name looks like one volume of a RAR set (.rar or .r00).
func IsVolume(name string) bool {
	base := filepath.Base(name)
	return strings.EqualFold(filepath.Ext(base), ".rar") || oldStyle.MatchString(base)
}

// IsFirstVolume says a file name is the first volume of a set: name.rar, or
// name.part1.rar (any zero padding).
func IsFirstVolume(name string) bool {
	base := filepath.Base(name)
	if m := partStyle.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[2])
		return n == 1
	}
	return strings.EqualFold(filepath.Ext(base), ".rar")
}

// NextVolume gives the path of the volume after path ("" when the name does
// not follow a known pattern).
func NextVolume(path string) string {
	dir, base := filepath.Split(path)
	if m := partStyle.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[2])
		return dir + m[1] + fmt.Sprintf("%0*d", len(m[2]), n+1) + m[3]
	}
	if strings.EqualFold(filepath.Ext(base), ".rar") {
		r := "r"
		if base[len(base)-3] == 'R' {
			r = "R"
		}
		return dir + base[:len(base)-3] + r + "00"
	}
	if m := oldStyle.FindStringSubmatch(base); m != nil {
		letter := m[2]
		n, _ := strconv.Atoi(m[3])
		n++
		if n == 100 {
			n = 0
			if letter == "z" || letter == "Z" {
				return ""
			}
			letter = string(letter[0] + 1)
		}
		return dir + m[1] + letter + fmt.Sprintf("%02d", n)
	}
	return ""
}

// List reads the headers of the set that starts at first (its first volume)
// and gives the files in it. It writes nothing.
func List(first string) ([]Entry, error) { return run(first, "") }

// Extract unpacks the set that starts at first into dest (which must be a
// folder of its own that exists). A compressed or encrypted set is refused
// before anything is written.
func Extract(first, dest string) ([]Entry, error) {
	entries, err := run(first, "")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Encrypted {
			return nil, ErrEncrypted
		}
		if e.Compressed {
			return nil, ErrCompressed
		}
	}
	return run(first, dest)
}

// part is one file header with the data that follows it in its volume.
type part struct {
	name                    string
	dir, link               bool
	packed, unpacked        int64
	dataOff                 int64
	splitBefore, splitAfter bool
	compressed, encrypted   bool
}

type current struct {
	e        Entry
	declared int64
	skip     bool // a folder or link: its data is passed over
	split    bool // continues in the next volume
	out      *os.File
}

func run(first, dest string) (entries []Entry, err error) {
	var cur *current
	defer func() {
		if cur != nil && cur.out != nil {
			cur.out.Close()
		}
	}()
	visit := func(p part, vol *os.File) error {
		name, err := cleanName(p.name)
		if err != nil {
			return err
		}
		if p.splitBefore {
			if cur == nil || !cur.split || cur.e.Name != name {
				return corrupt("a volume continues a file that was not started")
			}
		} else {
			if cur != nil {
				return corrupt("a file ends before its last part")
			}
			if len(entries) >= maxEntries {
				return corrupt("too many files")
			}
			cur = &current{e: Entry{Name: name}, declared: p.unpacked, skip: p.dir || p.link}
			if dest != "" && !cur.skip && !p.compressed && !p.encrypted {
				target := filepath.Join(dest, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					return err
				}
				if cur.out, err = os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
					return err
				}
			}
		}
		cur.split = p.splitAfter
		cur.e.Compressed = cur.e.Compressed || p.compressed
		cur.e.Encrypted = cur.e.Encrypted || p.encrypted
		if !cur.skip {
			if p.compressed || p.encrypted {
				cur.e.Size = cur.declared
			} else {
				cur.e.Size += p.packed
			}
			if cur.out != nil {
				if _, err := io.Copy(cur.out, io.NewSectionReader(vol, p.dataOff, p.packed)); err != nil {
					return corrupt("a volume is cut short")
				}
			}
		}
		if !p.splitAfter {
			if cur.out != nil {
				if err := cur.out.Close(); err != nil {
					return err
				}
				cur.out = nil
			}
			if !cur.skip {
				if !cur.e.Compressed && !cur.e.Encrypted && cur.e.Size < cur.declared {
					return corrupt("%s is shorter than its header says", name)
				}
				entries = append(entries, cur.e)
			}
			cur = nil
		}
		return nil
	}

	path := first
	for n := 0; ; n++ {
		if n >= maxVolumes {
			return nil, corrupt("too many volumes")
		}
		f, err := os.Open(path)
		if err != nil {
			if n > 0 && errors.Is(err, os.ErrNotExist) {
				return nil, corrupt("the volume %s is missing", filepath.Base(path))
			}
			return nil, err
		}
		more, err := walkVolume(f, visit)
		f.Close()
		if err != nil {
			return nil, err
		}
		if !more {
			break
		}
		if path = NextVolume(path); path == "" {
			return nil, corrupt("cannot tell which volume comes next")
		}
	}
	if cur != nil {
		return nil, corrupt("the last file is cut short")
	}
	return entries, nil
}

// cleanName turns a stored name into a safe relative path with "/" between
// folders, or refuses it.
func cleanName(raw string) (string, error) {
	if raw == "" || strings.ContainsRune(raw, 0) || len(raw) > maxNameLen {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, raw)
	}
	n := strings.ReplaceAll(raw, `\`, "/")
	if strings.HasPrefix(n, "/") || (len(n) >= 2 && n[1] == ':') || (runtime.GOOS == "windows" && strings.Contains(n, ":")) {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, raw)
	}
	var parts []string
	for _, p := range strings.Split(n, "/") {
		switch p {
		case "..":
			return "", fmt.Errorf("%w: %q", ErrUnsafePath, raw)
		case "", ".":
		default:
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, raw)
	}
	return strings.Join(parts, "/"), nil
}

var (
	sigRar4 = []byte("Rar!\x1a\x07\x00")
	sigRar5 = []byte("Rar!\x1a\x07\x01\x00")
)

// walkVolume reads the blocks of one volume, calling visit for each file
// part. It reports whether another volume follows.
func walkVolume(f *os.File, visit func(part, *os.File) error) (more bool, err error) {
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	size := fi.Size()
	sig := make([]byte, 8)
	n, _ := f.ReadAt(sig, 0)
	sig = sig[:n]
	switch {
	case len(sig) >= 8 && string(sig) == string(sigRar5):
		return walk5(f, size, 8, visit)
	case len(sig) >= 7 && string(sig[:7]) == string(sigRar4):
		return walk4(f, size, 7, visit)
	}
	return false, corrupt("%s is not a RAR file", filepath.Base(f.Name()))
}

func walk4(f *os.File, size, pos int64, visit func(part, *os.File) error) (bool, error) {
	lastSplit := false
	for pos+7 <= size {
		var h [7]byte
		if _, err := f.ReadAt(h[:], pos); err != nil {
			return false, corrupt("a header cannot be read")
		}
		typ := h[2]
		flags := binary.LittleEndian.Uint16(h[3:5])
		hsize := int64(binary.LittleEndian.Uint16(h[5:7]))
		if hsize < 7 || pos+hsize > size {
			return false, corrupt("a header is cut short")
		}
		buf := make([]byte, hsize)
		if _, err := f.ReadAt(buf, pos); err != nil {
			return false, corrupt("a header cannot be read")
		}
		var add int64
		if flags&0x8000 != 0 || typ == 0x74 {
			if hsize < 11 {
				return false, corrupt("a header is too short")
			}
			add = int64(binary.LittleEndian.Uint32(buf[7:11]))
		}
		switch typ {
		case 0x73: // main header
			if flags&0x0080 != 0 {
				return false, ErrEncrypted
			}
		case 0x74: // file header
			if hsize < 32 {
				return false, corrupt("a file header is too short")
			}
			p := part{packed: add, unpacked: int64(binary.LittleEndian.Uint32(buf[11:15]))}
			off := int64(32)
			if flags&0x100 != 0 {
				if hsize < 40 {
					return false, corrupt("a file header is too short")
				}
				p.packed |= int64(binary.LittleEndian.Uint32(buf[32:36])) << 32
				p.unpacked |= int64(binary.LittleEndian.Uint32(buf[36:40])) << 32
				off = 40
			}
			add = p.packed
			nameLen := int64(binary.LittleEndian.Uint16(buf[26:28]))
			if p.packed < 0 || p.packed > maxDataBytes || p.unpacked < 0 || p.unpacked > maxDataBytes || off+nameLen > hsize {
				return false, corrupt("a file header does not add up")
			}
			name := buf[off : off+nameLen]
			if flags&0x200 != 0 {
				if i := indexByte(name, 0); i >= 0 {
					name = name[:i]
				}
			}
			p.name = string(name)
			attr := binary.LittleEndian.Uint32(buf[28:32])
			p.dir = flags&0xE0 == 0xE0
			p.link = buf[15] == 3 && attr&0xF000 == 0xA000 // made on Unix, a symlink
			p.splitBefore, p.splitAfter = flags&0x01 != 0, flags&0x02 != 0
			p.encrypted = flags&0x04 != 0
			p.compressed = buf[25] != 0x30
			p.dataOff = pos + hsize
			if p.dataOff+p.packed > size {
				return false, corrupt("a volume is cut short")
			}
			lastSplit = p.splitAfter
			if err := visit(p, f); err != nil {
				return false, err
			}
		case 0x7B: // end of archive
			return flags&0x0001 != 0 || lastSplit, nil
		}
		pos += hsize + add
		if pos > size {
			return false, corrupt("a volume is cut short")
		}
	}
	return lastSplit, nil
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// reader5 reads the variable-length numbers and fixed fields of a RAR 5 header.
type reader5 struct {
	b   []byte
	bad bool
}

func (r *reader5) vint() uint64 {
	var v uint64
	for i := 0; i < 10; i++ {
		if len(r.b) == 0 {
			r.bad = true
			return 0
		}
		c := r.b[0]
		r.b = r.b[1:]
		v |= uint64(c&0x7F) << (7 * i)
		if c&0x80 == 0 {
			return v
		}
	}
	r.bad = true
	return 0
}

func (r *reader5) skip(n uint64) {
	if n > uint64(len(r.b)) {
		r.bad = true
		r.b = nil
		return
	}
	r.b = r.b[n:]
}

func (r *reader5) take(n uint64) []byte {
	if n > uint64(len(r.b)) {
		r.bad = true
		r.b = nil
		return nil
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func walk5(f *os.File, size, pos int64, visit func(part, *os.File) error) (bool, error) {
	lastSplit := false
	for pos+6 <= size {
		var head [14]byte
		n, _ := f.ReadAt(head[:], pos)
		sizeField := reader5{b: head[4:n]}
		hsize := sizeField.vint()
		sizeLen := int64(n-4) - int64(len(sizeField.b))
		if sizeField.bad || hsize == 0 || hsize > maxHeader {
			return false, corrupt("a header has a bad size")
		}
		end := pos + 4 + sizeLen + int64(hsize)
		if end > size {
			return false, corrupt("a header is cut short")
		}
		body := make([]byte, sizeLen+int64(hsize))
		if _, err := f.ReadAt(body, pos+4); err != nil {
			return false, corrupt("a header cannot be read")
		}
		if crc32.ChecksumIEEE(body) != binary.LittleEndian.Uint32(head[:4]) {
			return false, corrupt("a header fails its check")
		}
		r := reader5{b: body[sizeLen:]}
		typ, flags := r.vint(), r.vint()
		var extraSize, dataSize uint64
		if flags&0x1 != 0 {
			extraSize = r.vint()
		}
		if flags&0x2 != 0 {
			dataSize = r.vint()
		}
		if r.bad || extraSize > uint64(len(r.b)) || dataSize > uint64(maxDataBytes) {
			return false, corrupt("a header does not add up")
		}
		extra := r.b[uint64(len(r.b))-extraSize:]
		r.b = r.b[:uint64(len(r.b))-extraSize]
		switch typ {
		case 4: // encryption header: everything after it is encrypted
			return false, ErrEncrypted
		case 2: // file
			fileFlags := r.vint()
			unpacked := r.vint()
			r.vint() // attributes
			if fileFlags&0x2 != 0 {
				r.skip(4)
			}
			if fileFlags&0x4 != 0 {
				r.skip(4)
			}
			comp := r.vint()
			r.vint() // host OS
			nameLen := r.vint()
			if nameLen > maxNameLen {
				return false, corrupt("a file name is too long")
			}
			name := r.take(nameLen)
			if r.bad || unpacked > uint64(maxDataBytes) {
				return false, corrupt("a file header does not add up")
			}
			p := part{name: string(name), dir: fileFlags&0x1 != 0, packed: int64(dataSize), unpacked: int64(unpacked),
				dataOff: end, splitBefore: flags&0x8 != 0, splitAfter: flags&0x10 != 0, compressed: (comp>>7)&7 != 0}
			x := reader5{b: extra}
			for len(x.b) > 0 && !x.bad {
				recSize := x.vint()
				rec := reader5{b: x.take(recSize)}
				switch rec.vint() {
				case 0x01:
					p.encrypted = true
				case 0x05:
					p.link = true // a link, not a file
				}
			}
			if x.bad {
				return false, corrupt("a file header does not add up")
			}
			if p.dataOff+p.packed > size {
				return false, corrupt("a volume is cut short")
			}
			lastSplit = p.splitAfter
			if err := visit(p, f); err != nil {
				return false, err
			}
		case 5: // end of archive
			endFlags := r.vint()
			return endFlags&0x1 != 0 || lastSplit, nil
		}
		pos = end + int64(dataSize)
		if pos > size {
			return false, corrupt("a volume is cut short")
		}
	}
	return lastSplit, nil
}
