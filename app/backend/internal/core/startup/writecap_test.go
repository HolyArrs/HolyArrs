package startup

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecideWriteCap(t *testing.T) {
	for _, c := range []struct {
		name, env string
		limit     time.Duration
		odd       bool
	}{
		{"empty", "", 0, false},
		{"blank", "   ", 0, false},
		{"seconds", "90s", 90 * time.Second, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"spaces", " 2m ", 2 * time.Minute, false},
		{"bare seconds", "90", 90 * time.Second, false},
		{"default", "20s", 20 * time.Second, false},
		{"bare default", "20", 20 * time.Second, false},
		{"top", "1h", time.Hour, false},
		{"bare top", "3600", time.Hour, false},
		{"mixed", "1m30s", 90 * time.Second, false},
		{"too low", "5s", 0, true},
		{"just under", "19s", 0, true},
		{"bare too low", "19", 0, true},
		{"tiny", "1ms", 0, true},
		{"zero", "0", 0, true},
		{"zero duration", "0s", 0, true},
		{"negative", "-30s", 0, true},
		{"bare negative", "-90", 0, true},
		{"too high", "2h", time.Hour, true},
		{"just over", "1h0m1s", time.Hour, true},
		{"bare too high", "3601", time.Hour, true},
		{"huge bare", "36028797018963973", time.Hour, true},
		{"int64 max", "9223372036854775807", time.Hour, true},
		{"past int64", "99999999999999999999", 0, true},
		{"huge duration", "2562047h", time.Hour, true},
		{"word", "lots", 0, true},
		{"spaced unit", "90 s", 0, true},
		{"upper unit", "90S", 0, true},
		{"sec word", "60 sec", 0, true},
		{"exponent", "1e9", 0, true},
		{"hex", "0x5a", 0, true},
		{"symbol", "∞", 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			limit, odd := DecideWriteCap(c.env)
			if limit != c.limit || odd != c.odd {
				t.Errorf("DecideWriteCap(%q) = %v, %v; want %v, %v", c.env, limit, odd, c.limit, c.odd)
			}
		})
	}
}

func TestWriteCapLogsOnce(t *testing.T) {
	for _, c := range []struct {
		env, want string
		limit     time.Duration
	}{
		{"", "", 0},
		{"20s", "", 20 * time.Second},
		{"5m", "write cap set to 5m by HOLYARRS_WRITE_CAP", 5 * time.Minute},
		{"90", "write cap set to 1m30s by HOLYARRS_WRITE_CAP", 90 * time.Second},
		{"1h", "write cap set to 1h by HOLYARRS_WRITE_CAP", time.Hour},
		{"5s", "keeping the 20s write cap", 0},
		{"lots", "keeping the 20s write cap", 0},
		{"3h", "is over 1h; write cap set to 1h", time.Hour},
	} {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		t.Setenv(WriteCapEnv, c.env)
		got := WriteCap()
		log.SetOutput(os.Stderr)
		out := buf.String()
		if got != c.limit {
			t.Errorf("%q: cap %s, want %s", c.env, got, c.limit)
		}
		if n := strings.Count(out, "\n"); (c.want == "" && n != 0) || (c.want != "" && (n != 1 || !strings.Contains(out, c.want))) {
			t.Errorf("%q: log %q, want one line with %q", c.env, out, c.want)
		}
	}
}
