package database

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

func shareAs(kind string) func(string) (bool, string, error) {
	return func(string) (bool, string, error) { return true, kind, nil }
}

func noShare(string) (bool, string, error) { return false, "", nil }

// The refusal carries its kind as a type, so the start-up page can word
// it without reading the message, and errors.Is keeps working.
func TestNetShareErrorUnwrapsToErrNetworkShare(t *testing.T) {
	var err error = &NetShareError{Kind: "9p"}
	if !errors.Is(err, ErrNetworkShare) {
		t.Fatal("NetShareError must unwrap to ErrNetworkShare")
	}
}

func TestOpenRefusesWithATypedShareError(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(dir, Options{NetworkShare: shareAs("9p")})
	if !errors.Is(err, ErrNetworkShare) {
		t.Fatalf("got %v, want ErrNetworkShare", err)
	}
	var ns *NetShareError
	if !errors.As(err, &ns) || ns.Kind != "9p" {
		t.Fatalf("errors.As NetShareError: %v (%#v)", err, ns)
	}
	msg := err.Error()
	if !strings.Contains(msg, dir) {
		t.Errorf("the log line keeps the path: %q", msg)
	}
	if !strings.Contains(msg, files.ShareFix("9p")) {
		t.Errorf("the message carries the fix for 9p: %q", msg)
	}
	if strings.Contains(msg, "move the config folder to local storage") {
		t.Errorf("old advice (C: is local storage to a Windows user): %q", msg)
	}
	if _, statErr := os.Stat(filepath.Join(dir, MainFile)); statErr == nil {
		t.Fatal("database file created on a shared folder")
	}
}

// Docker Desktop 4.74-4.96 reports Mac binds as fakeowner, where locks are
// broken: refuse it like a share, with Desktop wording.
func TestOpenRefusesDesktopFakeowner(t *testing.T) {
	_, err := Open(t.TempDir(), Options{NetworkShare: shareAs("fakeowner")})
	if !errors.Is(err, ErrNetworkShare) {
		t.Fatalf("got %v, want ErrNetworkShare", err)
	}
	if !strings.Contains(err.Error(), "Docker Desktop") {
		t.Fatalf("Desktop wording: %v", err)
	}
}

// HOLYARRS_ALLOW_DESKTOP_FOLDER lifts only the fakeowner refusal: never
// FUSE (Unraid, mergerfs), 9p or a real network share.
func TestDesktopOverrideIsNarrow(t *testing.T) {
	t.Setenv("HOLYARRS_ALLOW_DESKTOP_FOLDER", "1")
	s, err := Open(t.TempDir(), Options{NetworkShare: shareAs("fakeowner")})
	if err != nil {
		t.Fatalf("override set, fakeowner refused: %v", err)
	}
	_ = s.Close()
	for _, kind := range []string{"fuse", "9p", "nfs", "smb"} {
		if _, err := Open(t.TempDir(), Options{NetworkShare: shareAs(kind)}); !errors.Is(err, ErrNetworkShare) {
			t.Errorf("%s: override must not lift it, got %v", kind, err)
		}
	}
}

// A config folder in memory (tmpfs, ramfs: Unraid's root when the pool
// path does not exist) is wiped at reboot: refuse it, with its own error.
func TestOpenRefusesConfigInMemory(t *testing.T) {
	for _, kind := range []string{"tmpfs", "ramfs"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			_, err := Open(dir, Options{NetworkShare: shareAs(kind)})
			if !errors.Is(err, ErrInMemory) {
				t.Fatalf("got %v, want ErrInMemory", err)
			}
			if errors.Is(err, ErrNetworkShare) {
				t.Errorf("memory is not a network share: %v", err)
			}
			if !strings.Contains(err.Error(), "HOLYARRS_CONFIG_IN_MEMORY") {
				t.Errorf("names the override: %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(dir, MainFile)); statErr == nil {
				t.Fatal("database file created in memory")
			}
		})
	}
}

func TestConfigInMemoryOverride(t *testing.T) {
	t.Setenv("HOLYARRS_CONFIG_IN_MEMORY", "yes")
	s, err := Open(t.TempDir(), Options{NetworkShare: shareAs("tmpfs")})
	if err != nil {
		t.Fatalf("override set, tmpfs refused: %v", err)
	}
	_ = s.Close()
	// The override is only for memory, never for a share.
	if _, err := Open(t.TempDir(), Options{NetworkShare: shareAs("fuse")}); !errors.Is(err, ErrNetworkShare) {
		t.Fatalf("memory override lifted a FUSE refusal: %v", err)
	}
}

// Doctor reads the schema version with the same query the migrator uses.
func TestSchemaVersionMatchesTheMigrator(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Options{NetworkShare: noShare})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	db, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	have, err := SchemaVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	know, _ := SchemaVersions()
	if have != know || know == 0 {
		t.Fatalf("SchemaVersion = %d, SchemaVersions = %d", have, know)
	}
}
