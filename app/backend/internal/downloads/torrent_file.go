package downloads

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strconv"
)

func torrentFileHash(b []byte) (string, error) {
	bad := errors.New("the Source did not return a valid torrent file")
	if len(b) < 2 || b[0] != 'd' {
		return "", bad
	}
	var value func(int, int) (int, error)
	str := func(pos int) (int, int, error) {
		if pos >= len(b) {
			return 0, 0, bad
		}
		off := bytes.IndexByte(b[pos:], ':')
		if off <= 0 || off > 10 {
			return 0, 0, bad
		}
		for _, c := range b[pos : pos+off] {
			if c < '0' || c > '9' {
				return 0, 0, bad
			}
		}
		n, err := strconv.ParseUint(string(b[pos:pos+off]), 10, 32)
		start := pos + off + 1
		if err != nil || n > uint64(len(b)-start) {
			return 0, 0, bad
		}
		return start, start + int(n), nil
	}
	value = func(pos, depth int) (int, error) {
		if pos >= len(b) || depth > 32 {
			return 0, bad
		}
		switch b[pos] {
		case 'i':
			off := bytes.IndexByte(b[pos+1:], 'e')
			if off <= 0 || off > 20 {
				return 0, bad
			}
			if _, err := strconv.ParseInt(string(b[pos+1:pos+1+off]), 10, 64); err != nil {
				return 0, bad
			}
			return pos + off + 2, nil
		case 'l', 'd':
			dict := b[pos] == 'd'
			pos++
			for pos < len(b) && b[pos] != 'e' {
				var err error
				if dict {
					_, pos, err = str(pos)
					if err != nil {
						return 0, err
					}
				}
				pos, err = value(pos, depth+1)
				if err != nil {
					return 0, err
				}
			}
			if pos >= len(b) {
				return 0, bad
			}
			return pos + 1, nil
		default:
			_, end, err := str(pos)
			return end, err
		}
	}
	pos := 1
	var info []byte
	for pos < len(b) && b[pos] != 'e' {
		start, end, err := str(pos)
		if err != nil {
			return "", err
		}
		key := b[start:end]
		pos = end
		end, err = value(pos, 1)
		if err != nil {
			return "", err
		}
		if bytes.Equal(key, []byte("info")) {
			if info != nil || b[pos] != 'd' {
				return "", bad
			}
			info = b[pos:end]
		}
		pos = end
	}
	if pos != len(b)-1 || b[pos] != 'e' || len(info) == 0 {
		return "", bad
	}
	h := sha1.Sum(info)
	return hex.EncodeToString(h[:]), nil
}
