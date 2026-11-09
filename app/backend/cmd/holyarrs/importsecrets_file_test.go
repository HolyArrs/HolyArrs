package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A link to the secrets file is refused (the file must be named itself).
func TestReadSecretsFileRefusesLinks(t *testing.T) {
	real := secretsFile(t, cliFile)
	link := filepath.Join(t.TempDir(), "link.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	_, err := readSecretsFile(link, true, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "link") {
		t.Fatalf("link accepted: %v", err)
	}
	if _, err := readSecretsFile(real, true, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
}
