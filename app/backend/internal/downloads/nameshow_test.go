package downloads

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDownloaderTestShowsWhatTheNameTurnedInto(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cases := []struct{ base, want string }{
		{"http://holyarrs-nameshow-test.home.arpa:8080", "holyarrs-nameshow-test.home.arpa → not found. HolyArrs can't find this name. Use the IP instead."},
		{"http://localhost:" + itoaInt(port), "→ refused on port " + itoaInt(port) + ". The name works, but nothing answers on that port. Check the port."},
	}
	for _, tc := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := NewSabnzbd(tc.base, sabKey, Client(tc.base, true)).Health(ctx)
		cancel()
		if err == nil {
			t.Fatal("the test passed")
		}
		if _, text := explainDownloader(err, tc.base, true); !strings.Contains(text, tc.want) {
			t.Fatalf("%s: text %q, want %q", tc.base, text, tc.want)
		}
	}
}
