package plugins

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPluginLogPrefixesLines(t *testing.T) {
	var buf safeBuf
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(prev); log.SetFlags(flags) }()

	w, err := pluginLog("tailscale")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("hello\npart"))
	_, _ = w.Write([]byte("ial\n"))
	_ = w.Close()
	want := "plugin tailscale: hello\nplugin tailscale: partial\n"
	deadline := time.Now().Add(2 * time.Second)
	for buf.String() != want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := buf.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

type safeBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *safeBuf) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }

// A line over the limit is cut, and the lines after it are still logged.
func TestPluginLogCutsLongLineKeepsLogging(t *testing.T) {
	var buf safeBuf
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(prev); log.SetFlags(flags) }()

	long := strings.Repeat("x", 70<<10)
	logLines("p", strings.NewReader(long+"\nhello\n"), time.Now)
	out := buf.String()
	if !strings.Contains(out, "[line cut]") || !strings.HasSuffix(out, "plugin p: hello\n") {
		t.Fatalf("log tail: %q", out[max(0, len(out)-80):])
	}
	if len(out) > maxLogLine+200 {
		t.Fatalf("long line not cut: %d bytes", len(out))
	}
}

// A chatty plugin logs at most logBurst lines per logEvery; the rest are
// counted once.
func TestPluginLogRateLimited(t *testing.T) {
	var buf safeBuf
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(prev); log.SetFlags(flags) }()

	at := time.Unix(1000, 0)
	logLines("p", strings.NewReader(strings.Repeat("spam\n", logBurst+50)), func() time.Time { return at })
	out := buf.String()
	if n := strings.Count(out, "plugin p: spam\n"); n != logBurst {
		t.Fatalf("logged %d lines, want %d", n, logBurst)
	}
	if !strings.Contains(out, "(50 more lines not logged") {
		t.Fatalf("dropped count missing: %q", out[max(0, len(out)-120):])
	}
}
