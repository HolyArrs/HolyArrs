package health

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
)

func TestRedactBundleHeadersAndPairs(t *testing.T) {
	for _, c := range []struct{ in, secret string }{
		{"Authorization: Bearer abc", "abc"},
		{"x-plex-token=zzz", "zzz"},
		{`"password":"p@ss"`, "p@ss"},
		{"apikey='k'", "'k'"},
	} {
		out := RedactBundle(c.in)
		if strings.Contains(out, c.secret) || !strings.Contains(out, logs.Hidden) {
			t.Errorf("%q -> %q", c.in, out)
		}
	}
	for _, in := range []string{"token: " + logs.Hidden, "folders: 4 ok\nfix: run chown"} {
		if out := RedactBundle(in); out != in {
			t.Errorf("%q changed to %q", in, out)
		}
	}
}
