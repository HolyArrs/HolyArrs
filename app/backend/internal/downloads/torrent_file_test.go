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
