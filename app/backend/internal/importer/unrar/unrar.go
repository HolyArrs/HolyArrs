package unrar

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Limits that keep a hostile set from using unbounded time or memory.
const (
	maxVolumes   = 2000
	maxEntries   = 100000
	maxNameLen   = 4096
	maxHeader    = 1 << 20
	maxDataBytes = int64(1) << 50
)

// maxServiceData caps a service block (comment, quick open data, stream). Real
// writers keep these small, so a big one may hide data and is refused.
const maxServiceData = 1 << 20

var (
	// ErrCompressed means an entry uses RAR compression.
	ErrCompressed = errors.New("the RAR files use compression")
	// ErrEncrypted means an entry or a header is password protected.
	ErrEncrypted = errors.New("the archive files are password protected")
	// ErrUnsafePath means an entry would land outside the target folder.
	ErrUnsafePath = errors.New("an archive entry has an unsafe path")
	// ErrCorrupt means the set is damaged, cut short or missing a volume.
	ErrCorrupt = errors.New("the archive files are damaged or incomplete")
	// ErrUnrecognised means the set holds a link, special file or block this
	// package does not understand.
	ErrUnrecognised = errors.New("the archive files hold something that is not understood")
	// ErrOldFormat means the file is a RAR from before RAR 1.5 (signature "RE~^").
	ErrOldFormat = errors.New("the RAR files use the RAR format from before RAR 1.5")
)

var (
	// partStyle matches name.partNN.rar and oldStyle matches name.r00 style names.
	partStyle = regexp.MustCompile(`(?i)^(.*\.part)(\d{1,6})(\.rar)$`)
	oldStyle  = regexp.MustCompile(`(?i)^(.*\.)([a-z])(\d\d)$`)
)

var (
	// sigRar4 and sigRar5 are the signatures that start a volume.
	sigRar4 = []byte("Rar!\x1a\x07\x00")
	sigRar5 = []byte("Rar!\x1a\x07\x01\x00")
)

// Entry is one file in a set. Folders and links are not listed.
type Entry struct {
	Name       string // cleaned path with "/" between folders
	Size       int64  // bytes once unpacked
	Compressed bool
	Encrypted  bool
	Link       bool // never unpacked
}

// part is one file header plus where its data sits in its volume.
type part struct {
	name                    string
	dir, link               bool
	packed, unpacked        int64
	dataOff                 int64
	splitBefore, splitAfter bool
	compressed, encrypted   bool
	crc                     uint32
	hasCRC                  bool
}

// current tracks the file being read while its parts arrive volume by volume.
type current struct {
	e        Entry
	declared int64
	skip     bool
	split    bool
	out      *os.File
	whole    hash.Hash32
}

// reader5 reads the variable-length numbers and fixed fields of a RAR 5 header.
type reader5 struct {
	b   []byte
	bad bool
}

// vint reads one variable-length number. Reading past the end sets bad.
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

// skip drops n bytes. Reading past the end sets bad.
func (r *reader5) skip(n uint64) {
	if n > uint64(len(r.b)) {
		r.bad = true
		r.b = nil
		return
	}
	r.b = r.b[n:]
}

// take returns the next n bytes. Reading past the end sets bad.
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

// List reads the headers of the set that starts at first and returns its files.
// It writes nothing.
func List(first string) ([]Entry, error) { return run(first, "") }

// Extract unpacks the set that starts at first into dest, which must be an
// existing folder of its own. Links, encrypted and compressed sets are refused
// before anything is written.
func Extract(first, dest string) ([]Entry, error) {
	entries, err := run(first, "")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Link {
			return nil, ErrUnrecognised
		}
		if e.Encrypted {
			return nil, ErrEncrypted
		}
		if e.Compressed {
			return nil, ErrCompressed
		}
	}
	return run(first, dest)
}

// IsVolume reports whether name looks like one volume of a RAR set.
func IsVolume(name string) bool {
	base := filepath.Base(name)
	return strings.EqualFold(filepath.Ext(base), ".rar") || oldStyle.MatchString(base)
}

// IsFirstVolume reports whether name is the first volume of a set.
func IsFirstVolume(name string) bool {
	base := filepath.Base(name)
	if m := partStyle.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[2])
		return n == 1
	}
	return strings.EqualFold(filepath.Ext(base), ".rar")
}

// NextVolume returns the path of the volume after path, or "" when the name
// follows no known pattern.
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

// CleanName turns a stored name into a safe relative path, or refuses it. The
// other archive readers share it so every format applies the same rule.
func CleanName(raw string) (string, error) { return cleanName(raw) }

// cleanName rejects empty, absolute, drive, NUL, overlong and ".." names.
// Backslashes count as separators because RAR writers on Windows use them.
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

// run walks every volume of the set. With a dest it also writes the files, and
// it checks each file against the CRC in its header.
func run(first, dest string) (entries []Entry, err error) {
	// Writes go through os.Root so a folder that is a link cannot lead
	// a file outside dest.
	var root *os.Root
	if dest != "" {
		if root, err = os.OpenRoot(dest); err != nil {
			return nil, err
		}
		defer root.Close()
	}
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
			cur = &current{e: Entry{Name: name, Link: p.link}, declared: p.unpacked, skip: p.dir || p.link, whole: crc32.NewIEEE()}
			if dest != "" && !cur.skip && !p.compressed && !p.encrypted {
				if i := strings.LastIndex(name, "/"); i > 0 {
					if err := root.MkdirAll(name[:i], 0o777); err != nil {
						return err
					}
				}
				if cur.out, err = root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666); err != nil {
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
				// Some writers put the CRC of the last part alone, so either sum may match.
				partSum := crc32.NewIEEE()
				if _, err := io.Copy(io.MultiWriter(cur.out, cur.whole, partSum), io.NewSectionReader(vol, p.dataOff, p.packed)); err != nil {
					return corrupt("a volume is cut short")
				}
				if !p.splitAfter && p.hasCRC && p.crc != cur.whole.Sum32() && p.crc != partSum.Sum32() {
					return corrupt("%s fails its checksum", name)
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
			if cur.e.Link {
				entries = append(entries, cur.e)
			} else if !cur.skip {
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

// corrupt wraps ErrCorrupt with a detail.
func corrupt(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, a...))
}

// walkVolume checks the signature, reads one volume and reports whether
// another volume follows.
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
	if len(sig) >= 4 && string(sig[:4]) == "RE~^" {
		return false, fmt.Errorf("%w: %s", ErrOldFormat, filepath.Base(f.Name()))
	}
	return false, corrupt("%s is not a RAR file", filepath.Base(f.Name()))
}

// walk4 reads the blocks of a RAR 4 volume and calls visit for each file part.
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
		// marker, comment, av, old sub, recovery and sign blocks carry nothing we use
		case 0x72, 0x75, 0x76, 0x77, 0x78, 0x79:
		case 0x7A:
			if add > maxServiceData {
				return false, ErrUnrecognised
			}
		case 0x73:
			if flags&0x0080 != 0 {
				return false, ErrEncrypted
			}
		case 0x74:
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
			p.name = string(name)
			if flags&0x200 != 0 {
				if i := indexByte(name, 0); i >= 0 {
					plain, err := plainAndUnicode(name[:i], name[i+1:])
					if err != nil {
						return false, err
					}
					p.name = plain
				}
			}
			attr := binary.LittleEndian.Uint32(buf[28:32])
			p.dir = flags&0xE0 == 0xE0
			// Host byte 3 means the writer was Unix, so attr holds a Unix mode.
			link, err := unixKind(buf[15] == 3, attr, &p.dir)
			if err != nil {
				return false, err
			}
			p.link = link
			p.crc, p.hasCRC = binary.LittleEndian.Uint32(buf[16:20]), true
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
		// the end block says whether the set goes on in another volume
		case 0x7B:
			return flags&0x0001 != 0 || lastSplit, nil
		default:
			return false, ErrUnrecognised
		}
		pos += hsize + add
		if pos > size {
			return false, corrupt("a volume is cut short")
		}
	}
	return lastSplit, nil
}

// walk5 reads the blocks of a RAR 5 volume and calls visit for each file part.
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
		// everything after an encryption header is encrypted
		case 4:
			return false, ErrEncrypted
		case 2:
			fileFlags := r.vint()
			unpacked := r.vint()
			attr := r.vint()
			if fileFlags&0x2 != 0 {
				r.skip(4)
			}
			var crc uint32
			if fileFlags&0x4 != 0 {
				if c := r.take(4); len(c) == 4 {
					crc = binary.LittleEndian.Uint32(c)
				}
			}
			comp := r.vint()
			host := r.vint()
			nameLen := r.vint()
			if nameLen > maxNameLen {
				return false, corrupt("a file name is too long")
			}
			name := r.take(nameLen)
			if r.bad || unpacked > uint64(maxDataBytes) {
				return false, corrupt("a file header does not add up")
			}
			p := part{name: string(name), dir: fileFlags&0x1 != 0, packed: int64(dataSize), unpacked: int64(unpacked),
				dataOff: end, splitBefore: flags&0x8 != 0, splitAfter: flags&0x10 != 0, compressed: (comp>>7)&7 != 0,
				crc: crc, hasCRC: fileFlags&0x4 != 0}
			// Host 1 means Unix.
			link, err := unixKind(host == 1, uint32(attr), &p.dir)
			if err != nil {
				return false, err
			}
			p.link = link
			x := reader5{b: extra}
			for len(x.b) > 0 && !x.bad {
				recSize := x.vint()
				rec := reader5{b: x.take(recSize)}
				switch rec.vint() {
				case 0x01:
					p.encrypted = true
				case 0x05:
					p.link = true
				case 0x02, 0x03, 0x04, 0x06, 0x07:
				default:
					return false, ErrUnrecognised
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
		case 5:
			endFlags := r.vint()
			return endFlags&0x1 != 0 || lastSplit, nil
		case 1:
		case 3:
			if dataSize > maxServiceData {
				return false, ErrUnrecognised
			}
		default:
			return false, ErrUnrecognised
		}
		pos = end + int64(dataSize)
		if pos > size {
			return false, corrupt("a volume is cut short")
		}
	}
	return lastSplit, nil
}

// unixKind reads the file type from a Unix mode. Only plain files, folders and
// links are allowed; a device, pipe or socket is refused.
func unixKind(unix bool, attr uint32, dir *bool) (link bool, err error) {
	if !unix {
		return false, nil
	}
	switch attr & 0xF000 {
	case 0, 0x8000:
		return false, nil
	case 0x4000:
		*dir = true
		return false, nil
	case 0xA000:
		return true, nil
	}
	return false, ErrUnrecognised
}

// plainAndUnicode picks the name of a RAR 4 entry stored twice. Tools read the
// Unicode form and this package the plain one, so an ASCII name must match
// both or the set is refused. A non-ASCII plain name is in an unknown code
// page, so the Unicode form is used if it is a safe path.
func plainAndUnicode(plain, enc []byte) (string, error) {
	uni, ok := decodeUnicodeName(plain, enc)
	if !ok {
		return "", ErrUnrecognised
	}
	ascii := true
	for _, c := range plain {
		ascii = ascii && c < 0x80
	}
	if ascii {
		if uni != string(plain) {
			return "", fmt.Errorf("%w: the two forms of a name differ", ErrUnrecognised)
		}
		return uni, nil
	}
	if _, err := cleanName(uni); err != nil {
		return "", err
	}
	return uni, nil
}

// decodeUnicodeName expands the RAR 4 name encoding: a high byte, then groups
// of four 2-bit commands that add a byte, a byte with the high byte, a 16-bit
// unit, or a run copied from the plain name.
func decodeUnicodeName(plain, enc []byte) (string, bool) {
	if len(enc) == 0 {
		return "", false
	}
	high := uint16(enc[0])
	pos := 1
	var out []uint16
	var flags byte
	bits := 0
	for pos < len(enc) {
		if bits == 0 {
			flags = enc[pos]
			pos++
			bits = 8
			if pos >= len(enc) {
				break
			}
		}
		switch flags >> 6 {
		case 0:
			out = append(out, uint16(enc[pos]))
			pos++
		case 1:
			out = append(out, uint16(enc[pos])|high<<8)
			pos++
		case 2:
			if pos+1 >= len(enc) {
				return "", false
			}
			out = append(out, uint16(enc[pos])|uint16(enc[pos+1])<<8)
			pos += 2
		case 3:
			n := int(enc[pos])
			pos++
			corr, shifted := byte(0), n&0x80 != 0
			if shifted {
				if pos >= len(enc) {
					return "", false
				}
				corr = enc[pos]
				pos++
				n &= 0x7F
			}
			for i := 0; i < n+2; i++ {
				if len(out) >= len(plain) {
					return "", false
				}
				u := uint16(plain[len(out)] + corr)
				if shifted {
					u |= high << 8
				}
				out = append(out, u)
			}
		}
		flags <<= 2
		bits -= 2
		if len(out) > maxNameLen {
			return "", false
		}
	}
	return string(utf16.Decode(out)), true
}

// indexByte returns the first index of c in b, or -1.
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
