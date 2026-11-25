package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The plain-HTTP fallback keys on tsnet's error text; fail loudly if a
// Tailscale update changes it.
func TestHTTPSOffTextMatchesTsnet(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "tailscale.com").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	src, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "tsnet", "tsnet.go"))
	if err != nil {
		t.Skipf("tsnet source: %v", err)
	}
	if !strings.Contains(string(src), httpsOffText) {
		t.Fatalf("tsnet no longer says %q; update httpsOff", httpsOffText)
	}
	if !httpsOff(errors.New("tsnet: "+httpsOffText+" to proceed.")) || httpsOff(errors.New("other")) || httpsOff(nil) {
		t.Fatal("httpsOff")
	}
}

func TestSavedName(t *testing.T) {
	d := t.TempDir()
	t.Setenv(envData, d)
	for in, want := range map[string]string{"den": "den", "Bad Name": "holyarrs", "-x": "holyarrs", "../etc": "holyarrs", "": "holyarrs"} {
		os.WriteFile(filepath.Join(d, "hostname"), []byte(in+"\n"), 0o600)
		if got := savedName("holyarrs"); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
