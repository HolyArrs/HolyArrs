package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/store"
)

const secretDir = "/srv/private-config-dir"

func shareErr(kind string) error {
	return fmt.Errorf("%w: %s; %s", &store.NetShareError{Kind: kind}, secretDir, store.ShareFix(kind))
}

// Refusals a person has to fix keep the page up instead of exiting into a
// restart loop (restart: unless-stopped would start the same refusal).
func TestStartupRefusalsNeedAPerson(t *testing.T) {
	for name, err := range map[string]error{
		"9p":           &store.NetShareError{Kind: "9p"},
		"wrapped fuse": shareErr("fuse"),
		"fakeowner":    shareErr("fakeowner"),
		"in memory":    fmt.Errorf("%w: %s", store.ErrInMemory, secretDir),
		"not writable": fmt.Errorf("%w: %s", store.ErrNotWritable, secretDir),
	} {
		if !needsPerson(err) {
			t.Errorf("%s: needsPerson = false", name)
		}
	}
	if needsPerson(errors.New("disk on fire")) {
		t.Error("an unknown error still exits")
	}
}

func TestPublicReasonForAShare(t *testing.T) {
	cases := []struct {
		kind string
		has  []string
	}{
		{"9p", []string{"9p", "named volume"}},
		{"fakeowner", []string{"Docker Desktop", "named volume"}},
		{"fuse", []string{"/mnt/<pool>/appdata"}},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			msg := publicReason(shareErr(c.kind))
			for _, w := range c.has {
				if !strings.Contains(msg, w) {
					t.Errorf("lacks %q: %s", w, msg)
				}
			}
			for _, bad := range []string{"restore", "see the log", secretDir, "cache"} {
				if strings.Contains(msg, bad) {
					t.Errorf("should not say %q: %s", bad, msg)
				}
			}
		})
	}
}

func TestPublicReasonForMemoryAndReadOnly(t *testing.T) {
	mem := publicReason(fmt.Errorf("%w: %s", store.ErrInMemory, secretDir))
	for _, w := range []string{"memory", "HOLYARRS_CONFIG_IN_MEMORY"} {
		if !strings.Contains(mem, w) {
			t.Errorf("in memory lacks %q: %s", w, mem)
		}
	}
	ro := publicReason(fmt.Errorf("%w: %s", store.ErrNotWritable, secretDir))
	if !strings.Contains(ro, "writ") {
		t.Errorf("read-only config: %s", ro)
	}
	for _, msg := range []string{mem, ro} {
		for _, bad := range []string{"restore", "see the log", secretDir} {
			if strings.Contains(msg, bad) {
				t.Errorf("should not say %q: %s", bad, msg)
			}
		}
	}
}

// The refusal shows the fix on the page and reports failed to healthz, with
// no go-back form.
func TestShareRefusalPage(t *testing.T) {
	srv := httptest.NewServer(refusedHandler(t.TempDir(), shareErr("9p"), make(chan struct{})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(b), `"failed"`) {
		t.Fatalf("healthz %d %s", resp.StatusCode, b)
	}
	resp, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(b)
	if !strings.Contains(page, "named volume") || strings.Contains(page, "one-time code") || strings.Contains(page, secretDir) {
		t.Fatalf("page: %s", page)
	}
}

// Forgot to map /config: one boxed log warning with the rescue, nothing
// when mapped or unsure. Start-up never refuses for it.
func TestUnmappedConfigLogLine(t *testing.T) {
	for _, kind := range []string{"anonymous", "none"} {
		msg := unmappedConfigWarning("/config", kind)
		if msg == "" || !strings.Contains(msg, "/config") || !strings.Contains(msg, "docker cp") {
			t.Errorf("%s: %q", kind, msg)
		}
	}
	if msg := unmappedConfigWarning("/config", ""); msg != "" {
		t.Errorf("mapped: want no warning, got %q", msg)
	}
}
