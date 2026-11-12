package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestDoctorNeverAsksTheServerUnlessServerMode(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	run := func() Report {
		return RunDoctor(context.Background(), DoctorOptions{ConfigDir: t.TempDir(), SavePath: t.TempDir(), MetadataURL: srv.URL, HTTP: srv.Client()})
	}
	text := func(r Report) string {
		var b strings.Builder
		for _, c := range r.Checks {
			if c.Section == "Network" {
				b.WriteString(string(c.Status) + " " + c.Text + "\n")
			}
		}
		return b.String()
	}
	for _, v := range []string{"off", "own"} {
		t.Setenv(metadata.EnvMode, v)
		if got := text(run()); !strings.Contains(got, "not asked") {
			t.Fatalf("%s: %s", v, got)
		}
	}
	t.Setenv(metadata.EnvMode, "ful")
	r := run()
	if got := text(r); !strings.Contains(got, `"ful"`) || !r.Failed() {
		t.Fatalf("typo: %s", got)
	}
	if hits.Load() != 0 {
		t.Fatalf("doctor asked the server %d times", hits.Load())
	}
	t.Setenv(metadata.EnvMode, "")
	run()
	if hits.Load() != 1 {
		t.Fatalf("server mode asks once, got %d", hits.Load())
	}
}
