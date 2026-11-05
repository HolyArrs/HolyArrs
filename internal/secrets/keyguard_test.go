package secrets

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// keyPrint is the frozen fingerprint format the doctor, the key kit and
// the Backups card show: the first 8 hex digits of SHA-256 over the 32 key
// bytes.
func keyPrint(key []byte) string {
	s := sha256.Sum256(key)
	return hex.EncodeToString(s[:])[:8]
}

// A config folder whose database already holds secrets but whose key file
// is gone must never get a fresh key: that key could not open anything, and
// the saved passwords would look lost for good. LoadKey refuses with a
// clear error and leaves the folder as it was.
func TestLoadKeyRefusesNewKeyWhenSecretsSaved(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, err := LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Put(context.Background(), "source_api_key", "lab-value"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, KeyFile)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKey(dir, "")
	if err == nil {
		t.Fatalf("made a new key (%d bytes) for a database that already holds secrets", len(got))
	}
	if !strings.Contains(err.Error(), KeyFile) || !strings.Contains(strings.ToLower(err.Error()), "master key") {
		t.Errorf("error should name the master key and %s in plain words: %v", KeyFile, err)
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a key file was written anyway: %v", err)
	}
}

// Only the check value (the vault was opened once, nothing else saved) is
// still a database tied to a key: a new key could not open it either.
func TestLoadKeyRefusesNewKeyWhenOnlyCheckValueSaved(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, err := LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), s, key); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, KeyFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(dir, ""); err == nil {
		t.Fatal("made a new key for a database tied to another key")
	}
}

// A database with no secrets yet (a fresh install) still gets its key made
// on first start, as today.
func TestLoadKeyStillMadeForDatabaseWithoutSecrets(t *testing.T) {
	dir := t.TempDir()
	_ = openStore(t, dir)
	k, err := LoadKey(dir, "")
	if err != nil || len(k) != KeySize {
		t.Fatalf("fresh database: key %d bytes, %v", len(k), err)
	}
	if _, err := os.Stat(filepath.Join(dir, KeyFile)); err != nil {
		t.Fatal(err)
	}
}

// The key kit the owner downloads is itself a key file: '#' comment lines
// (what it is, its fingerprint, how to put it back) and the key on its own
// line. Restoring means renaming it to holyarrs.key, or naming it in
// HOLYARRS_MASTER_KEY_FILE; Windows line endings from an edited copy work.
func TestKeyKitFileIsAValidKeyFile(t *testing.T) {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = byte(i*7 + 3)
	}
	kits := map[string]string{
		"base64": "# HolyArrs master key\n# fingerprint " + keyPrint(key) + "\n# rename this file to holyarrs.key and put it in /config\n" +
			base64.StdEncoding.EncodeToString(key) + "\n",
		"hex, CRLF":   "# HolyArrs master key\r\n# fingerprint " + keyPrint(key) + "\r\n" + hex.EncodeToString(key) + "\r\n",
		"blank lines": "\n# HolyArrs master key\n\n" + base64.StdEncoding.EncodeToString(key) + "\n\n# end\n",
	}
	for name, kit := range kits {
		t.Run(name, func(t *testing.T) {
			named := filepath.Join(t.TempDir(), "holyarrs-key-kit.key")
			if err := os.WriteFile(named, []byte(kit), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadKey(t.TempDir(), named)
			if err != nil || string(got) != string(key) {
				t.Fatalf("kit as HOLYARRS_MASTER_KEY_FILE: %x %v", got, err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, KeyFile), []byte(kit), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err = LoadKey(dir, "")
			if err != nil || string(got) != string(key) {
				t.Fatalf("kit renamed to %s: %x %v", KeyFile, got, err)
			}
		})
	}
	// Comments alone are not a key.
	named := filepath.Join(t.TempDir(), "only-comments.key")
	if err := os.WriteFile(named, []byte("# HolyArrs master key\n# nothing here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(t.TempDir(), named); err == nil {
		t.Fatal("a file with only comment lines was taken as a key")
	}
}

// A config folder whose name holds '#' or '?' must still be read as one
// path: otherwise SQLite stops the path there, opens some other file read
// and write, sees no secrets and a new key is made over the saved ones.
func TestLoadKeyRefusesNewKeyInFolderWithHashOrQuestion(t *testing.T) {
	for _, name := range []string{"holy#arrs", "holy%3Farrs", "holy%23arrs"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, name)
			s := openStore(t, dir)
			key, err := LoadKey(dir, "")
			if err != nil {
				t.Fatal(err)
			}
			v, err := Open(context.Background(), s, key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := v.Put(context.Background(), "source_api_key", "lab-value"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dir, KeyFile)); err != nil {
				t.Fatal(err)
			}
			if got, err := LoadKey(dir, ""); err == nil {
				t.Fatalf("made a new key (%d bytes) in %q for a database that holds secrets", len(got), name)
			} else if !errors.Is(err, ErrKeyMissing) {
				t.Fatalf("want ErrKeyMissing, got %v", err)
			}
			if err := CheckFile(filepath.Join(dir, "holyarrs.db"), make([]byte, KeySize)); !errors.Is(err, ErrWrongKey) {
				t.Fatalf("CheckFile in %q with another key: %v", name, err)
			}
			ents, _ := os.ReadDir(parent)
			if len(ents) != 1 {
				var names []string
				for _, e := range ents {
					names = append(names, e.Name())
				}
				t.Fatalf("stray files next to the config folder: %v", names)
			}
		})
	}
}
