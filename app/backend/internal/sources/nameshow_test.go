package sources

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestSourceTestShowsWhatTheNameTurnedInto(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	cases := []struct {
		url, proxy, want string
	}{
		// Absolute names (trailing dot), so a DNS search domain with a wildcard
		// record cannot make them resolve.
		{"http://holyarrs-nameshow-test.home.arpa.:9117/api", "", "holyarrs-nameshow-test.home.arpa. → not found. HolyArrs can't find this name. Use the IP instead."},
		{"http://localhost:" + port + "/api", "", "→ refused on port " + port + ". The name works, but nothing answers on that port. Check the port."},
		// Through a proxy the name is the proxy's to look up: no name step.
		{"http://holyarrs-nameshow-test.home.arpa.:9117/api", "http://127.0.0.1:1", ""},
	}
	for _, tc := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := New(Config{URL: tc.url, APIKey: "k", Protocol: "torrent", AllowLocal: true, Timeout: 500 * time.Millisecond}).Test(ctx)
		cancel()
		if err == nil {
			t.Fatal("the test passed")
		}
		src := domain.Source{URL: tc.url, AllowLocal: true, ProxyURL: tc.proxy}
		got := ExplainVia(tc.url, err, ProbeDial(src))
		if tc.want == "" {
			if strings.Contains(got, "→") {
				t.Fatalf("%s via proxy: %q", tc.url, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%s: %q, want %q", tc.url, got, tc.want)
		}
	}
}
