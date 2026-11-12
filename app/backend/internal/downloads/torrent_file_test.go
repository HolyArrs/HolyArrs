package downloads

import (
	"strings"
	"testing"
)

func TestTorrentFileBoundary(t *testing.T) {
	good := []byte("d4:infod6:lengthi1e4:name5:owned12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	if _, err := torrentFileHash(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<html>no</html>", "de", "d4:infoi1ee", string(good) + "trailing", "d4:infod4:name999999999999999999999:xe", strings.Repeat("l", 100) + strings.Repeat("e", 100)} {
		if _, err := torrentFileHash([]byte(bad)); err == nil {
			t.Fatal("accepted malformed torrent")
		}
	}
}

func TestTorrentPrivate(t *testing.T) {
	for in, want := range map[string]bool{
		"d4:infod6:lengthi1e4:name1:a7:privatei1eee":   true,
		"d4:infod6:lengthi1e4:name1:a7:privatei0eee":   false,
		"d4:infod6:lengthi1e4:name1:aee":               false,
		"d7:privatei1e4:infod6:lengthi1e4:name1:aee":   false,
		"d4:infod6:lengthi1e4:name14:a7:privatei1eeee": false,
		"not a torrent": false,
		"d8:announce3:x/y4:infod5:filesle4:name1:a7:privatei1eee": true,
	} {
		if got := torrentPrivate([]byte(in)); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}
