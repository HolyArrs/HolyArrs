package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// clearAddrEnv empties the listen-address variables for one test.
func clearAddrEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvAddr, EnvPort, "HOLYARRS_HEALTH_URL"} {
		t.Setenv(k, "")
		t.Setenv("FILE__"+k, "")
		_ = os.Unsetenv("FILE__" + k)
	}
}

func TestListenAddrFromEnv(t *testing.T) {
	for _, c := range []struct {
		name, addr, port string
		args             []string
		want             string
	}{
		{name: "default", want: ":6262"},
		{name: "port only", port: "8080", want: ":8080"},
		{name: "address only", addr: "127.0.0.1:8081", want: "127.0.0.1:8081"},
		{name: "address wins over port", addr: ":8082", port: "8083", want: ":8082"},
		{name: "flag wins over both", addr: ":8082", port: "8083", args: []string{"-addr", ":9000"}, want: ":9000"},
		{name: "flag wins over a bad variable", port: "nope", args: []string{"-addr", ":9001"}, want: ":9001"},
		{name: "IPv6 address", addr: "[::1]:8084", want: "[::1]:8084"},
	} {
		t.Run(c.name, func(t *testing.T) {
			clearAddrEnv(t)
			t.Setenv(EnvAddr, c.addr)
			t.Setenv(EnvPort, c.port)
			got, err := parseFlags(c.args, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if got.Addr != c.want {
				t.Fatalf("Addr = %q, want %q", got.Addr, c.want)
			}
		})
	}
}

func TestListenAddrBadValueStops(t *testing.T) {
	for _, c := range []struct{ key, val string }{
		{EnvPort, "abc"}, {EnvPort, "0"}, {EnvPort, "65536"}, {EnvPort, ":8080"},
		{EnvAddr, "8080"}, {EnvAddr, ":http"}, {EnvAddr, "host:99999"}, {EnvAddr, "::1:8080"},
	} {
		t.Run(c.key+"="+c.val, func(t *testing.T) {
			clearAddrEnv(t)
			t.Setenv(c.key, c.val)
			var out strings.Builder
			if _, err := parseFlags(nil, &out); err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(out.String(), c.key) {
				t.Fatalf("message %q does not name %s", out.String(), c.key)
			}
		})
	}
}

// A FILE__ variant goes through the generic loader like every other variable.
func TestListenPortFromFile(t *testing.T) {
	clearAddrEnv(t)
	ln := listenFree(t)
	served := serveHealth(t, ln)
	f := filepath.Join(t.TempDir(), "port")
	if err := os.WriteFile(f, []byte(strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILE__"+EnvPort, f)
	if got := healthcheckCmd(); got != 0 {
		t.Fatalf("health check with FILE__%s = %d, want 0", EnvPort, got)
	}
	if served.Load() == 0 {
		t.Fatal("the health check did not call the port from the file")
	}
}

func TestHealthcheckFollowsListenEnv(t *testing.T) {
	ln := listenFree(t)
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	served := serveHealth(t, ln)

	clearAddrEnv(t)
	t.Setenv(EnvPort, port)
	if got := healthcheckCmd(); got != 0 {
		t.Fatalf("health check with %s=%s = %d, want 0", EnvPort, port, got)
	}
	clearAddrEnv(t)
	t.Setenv(EnvAddr, "0.0.0.0:"+port)
	t.Setenv(EnvPort, "1")
	if got := healthcheckCmd(); got != 0 {
		t.Fatalf("health check with %s=0.0.0.0:%s = %d, want 0", EnvAddr, port, got)
	}
	if n := served.Load(); n != 2 {
		t.Fatalf("served %d checks, want 2", n)
	}
	// HOLYARRS_HEALTH_URL still wins.
	t.Setenv("HOLYARRS_HEALTH_URL", "http://127.0.0.1:1")
	if got := healthcheckCmd(); got != 1 {
		t.Fatalf("health check with a dead HOLYARRS_HEALTH_URL = %d, want 1", got)
	}
	// A bad value is unhealthy, not a check of the default port.
	clearAddrEnv(t)
	t.Setenv(EnvPort, "nope")
	if got := healthcheckCmd(); got != 1 {
		t.Fatalf("health check with a bad %s = %d, want 1", EnvPort, got)
	}
}

func TestHealthBase(t *testing.T) {
	for in, want := range map[string]string{
		":6262":            "http://127.0.0.1:6262",
		"0.0.0.0:8080":     "http://127.0.0.1:8080",
		"[::]:8080":        "http://127.0.0.1:8080",
		"192.168.1.5:8080": "http://192.168.1.5:8080",
		"[::1]:8080":       "http://[::1]:8080",
	} {
		if got := healthBase(in); got != want {
			t.Errorf("healthBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func listenFree(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

// serveHealth answers /healthz on ln and counts the calls.
func serveHealth(t *testing.T, ln net.Listener) *atomic.Int32 {
	t.Helper()
	n := new(atomic.Int32)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			n.Add(1)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(404)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return n
}
