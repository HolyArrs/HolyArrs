package downloads

import (
	"bytes"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

var errTorrent = errors.New("the Source did not return a valid torrent file")

type bval struct {
	kind byte
	s    []byte
	i    int64
	l    []bval
	d    []bkv
	raw  []byte
}

type bkv struct {
	key []byte
	val bval
}

func bdecode(b []byte, pos, depth int) (bval, int, error) {
	if pos >= len(b) || depth > 32 {
		return bval{}, 0, errTorrent
	}
	start := pos
	switch c := b[pos]; {
	case c == 'i':
		off := bytes.IndexByte(b[pos+1:], 'e')
		if off <= 0 || off > 20 {
			return bval{}, 0, errTorrent
		}
		n, err := strconv.ParseInt(string(b[pos+1:pos+1+off]), 10, 64)
		if err != nil {
			return bval{}, 0, errTorrent
		}
		end := pos + off + 2
		return bval{kind: 'i', i: n, raw: b[start:end]}, end, nil
	case c == 'l' || c == 'd':
		v := bval{kind: c}
		pos++
		for pos < len(b) && b[pos] != 'e' {
			var key bval
			var err error
			if c == 'd' {
				if key, pos, err = bdecode(b, pos, depth+1); err != nil || key.kind != 's' {
					return bval{}, 0, errTorrent
				}
			}
			var item bval
			if item, pos, err = bdecode(b, pos, depth+1); err != nil {
				return bval{}, 0, err
			}
			if c == 'd' {
				v.d = append(v.d, bkv{key.s, item})
			} else {
				v.l = append(v.l, item)
			}
		}
		if pos >= len(b) {
			return bval{}, 0, errTorrent
		}
		v.raw = b[start : pos+1]
		return v, pos + 1, nil
	case c >= '0' && c <= '9':
		off := bytes.IndexByte(b[pos:], ':')
		if off <= 0 || off > 10 {
			return bval{}, 0, errTorrent
		}
		n, err := strconv.ParseUint(string(b[pos:pos+off]), 10, 32)
		s := pos + off + 1
		if err != nil || n > uint64(len(b)-s) {
			return bval{}, 0, errTorrent
		}
		end := s + int(n)
		return bval{kind: 's', s: b[s:end], raw: b[start:end]}, end, nil
	}
	return bval{}, 0, errTorrent
}

func bstr(s []byte) []byte {
	return append(strconv.AppendInt(nil, int64(len(s)), 10), append([]byte{':'}, s...)...)
}

func homeHost(host string) bool {
	if strings.IndexFunc(host, func(r rune) bool { return r < 0x21 || r == 0x7f }) >= 0 {
		return true
	}
	host = strings.NewReplacer("。", ".", "．", ".", "｡", ".").Replace(host)
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if host == "" || metadataName(host) {
		return true
	}
	if ip, numeric := hostIP(host); ip != nil {
		return !outbound.PublicOnly(ip)
	} else if numeric {
		return true
	}
	return oddHost(host) || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local")
}

func homeURL(raw []byte) bool {
	u, err := url.Parse(string(raw))
	return err != nil || oddAuthority(string(raw)) || homeHost(u.Hostname())
}

func oddAuthority(s string) bool {
	i := strings.Index(s, "//")
	if i < 0 {
		return false
	}
	a := s[i+2:]
	if j := strings.IndexByte(a, '/'); j >= 0 {
		a = a[:j]
	}
	return strings.ContainsAny(a, "@?#\\")
}

func cleanTorrent(b []byte) ([]byte, int, error) {
	top, end, err := bdecode(b, 0, 0)
	if err != nil || top.kind != 'd' || end != len(b) {
		return nil, 0, errTorrent
	}
	dropped := 0
	var keepURLs func(v bval) (bval, bool)
	keepURLs = func(v bval) (bval, bool) {
		switch v.kind {
		case 's':
			if homeURL(v.s) {
				dropped++
				return v, false
			}
			return v, true
		case 'l':
			var out []bval
			for _, it := range v.l {
				if it.kind == 's' && homeURL(it.s) {
					dropped++
					continue
				}
				if it.kind == 'l' {
					if t, ok := keepURLs(it); ok {
						out = append(out, t)
					}
					continue
				}
				out = append(out, it)
			}
			return bval{kind: 'l', l: out}, len(out) > 0
		}
		return v, true
	}
	keepNodes := func(v bval) (bval, bool) {
		if v.kind != 'l' {
			return v, true
		}
		var out []bval
		for _, n := range v.l {
			if n.kind == 'l' && len(n.l) > 0 && n.l[0].kind == 's' && homeHost(string(n.l[0].s)) {
				dropped++
				continue
			}
			out = append(out, n)
		}
		return bval{kind: 'l', l: out}, len(out) > 0
	}
	var w bytes.Buffer
	w.WriteByte('d')
	for _, kv := range top.d {
		v, keep := kv.val, true
		switch string(kv.key) {
		case "announce", "announce-list", "url-list", "httpseeds":
			v, keep = keepURLs(v)
		case "nodes":
			v, keep = keepNodes(v)
		}
		if !keep {
			continue
		}
		w.Write(bstr(kv.key))
		w.Write(bencode(v))
	}
	w.WriteByte('e')
	if dropped == 0 {
		return b, 0, nil
	}
	return w.Bytes(), dropped, nil
}

func bencode(v bval) []byte {
	if v.raw != nil {
		return v.raw
	}
	switch v.kind {
	case 's':
		return bstr(v.s)
	case 'i':
		return []byte("i" + strconv.FormatInt(v.i, 10) + "e")
	case 'l':
		out := []byte{'l'}
		for _, it := range v.l {
			out = append(out, bencode(it)...)
		}
		return append(out, 'e')
	}
	out := []byte{'d'}
	for _, kv := range v.d {
		out = append(out, bstr(kv.key)...)
		out = append(out, bencode(kv.val)...)
	}
	return append(out, 'e')
}

func safeTorrent(b []byte) ([]byte, error) {
	h, err := torrentFileHash(b)
	if err != nil {
		return nil, err
	}
	clean, n, err := cleanTorrent(b)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return b, nil
	}
	if h2, err := torrentFileHash(clean); err != nil || h2 != h {
		return nil, errTorrent
	}
	return clean, nil
}
