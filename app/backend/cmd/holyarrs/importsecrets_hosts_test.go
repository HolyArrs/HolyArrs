package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportSecretsHostRules(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) bool
		host string
		want bool
	}{
		{"metadata", metadataHost, "169.254.169.254", true},
		{"metadata", metadataHost, "metadata.google.internal", true},
		{"metadata", metadataHost, "10.0.0.5", false},
		{"metadata", metadataHost, "169.254.1.2", true},
		{"metadata", metadataHost, "168.63.129.16", true},
		{"metadata", metadataHost, "fd20:ce::254", true},
		{"metadata", metadataHost, "fd00:c1::a9fe:a9fe", true},
		{"metadata", metadataHost, "fd00:a9fe:a9fe::1", true},
		{"metadata", metadataHost, "fd00:ec2::23", true},
		{"metadata", metadataHost, "64:ff9b::a9fe:a9fe", true},
		{"local", localHost, "10.0.0.5", true},
		{"local", localHost, "sonarr", true},
		{"local", localHost, "example.com", false},
		{"loopback", loopbackHost, "::1", true},
		{"loopback", loopbackHost, "localhost", true},
		{"loopback", loopbackHost, "example.com", false},
	}
	for _, c := range cases {
		if got := c.fn(c.host); got != c.want {
			t.Errorf("%sHost(%q) = %v, want %v", c.name, c.host, got, c.want)
		}
	}
}

func TestWipeRemovesFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secrets.json")
	if err := os.WriteFile(p, []byte(`{"key":"abc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := wipe(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
}
