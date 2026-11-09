package health

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// keySection is the doctor section for the master key.
const keySection = "Master key"

// keyPrint is the frozen fingerprint format: the first 8 hex digits of
// SHA-256 over the 32 key bytes.
func keyPrint(key []byte) string {
	s := sha256.Sum256(key)
	return hex.EncodeToString(s[:])[:8]
}

// keyedConfig makes a config folder whose database holds a secret, and
// returns the key that opens it.
func keyedConfig(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Put(context.Background(), "source_api_key", "lab-value"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, key
}

func runDoctor(t *testing.T, dir string) Report {
	t.Helper()
	t.Setenv(secrets.EnvKeyFile, "")
	return runDoctorEnv(t, dir)
}

func runDoctorEnv(t *testing.T, dir string) Report {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	t.Cleanup(srv.Close)
	return RunDoctor(context.Background(), DoctorOptions{Version: "9.9.9", ConfigDir: dir, SavePath: t.TempDir(), HTTP: srv.Client(),
		MetadataURL: srv.URL, Now: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)})
}

func keyChecks(r Report) []Check {
	var out []Check
	for _, c := range r.Checks {
		if c.Section == keySection {
			out = append(out, c)
		}
	}
	return out
}

func worstKey(t *testing.T, r Report) Check {
	t.Helper()
	cs := keyChecks(r)
	if len(cs) == 0 {
		t.Fatalf("no %q section in\n%s", keySection, r.Text())
	}
	w := cs[0]
	for _, c := range cs {
		if c.Status == StatusFail || (c.Status == StatusWarn && w.Status == StatusOK) {
			w = c
		}
	}
	return w
}

func noKeyText(t *testing.T, r Report, key []byte) {
	t.Helper()
	text := r.Text() + string(r.JSON())
	for _, form := range []string{base64.StdEncoding.EncodeToString(key), hex.EncodeToString(key)} {
		if strings.Contains(text, form) {
			t.Fatalf("doctor printed the master key:\n%s", text)
		}
	}
}

func TestDoctorKeyMatches(t *testing.T) {
	dir, key := keyedConfig(t)
	if runtime.GOOS != "windows" {
		_ = os.Chmod(filepath.Join(dir, secrets.KeyFile), 0o600)
	}
	r := runDoctor(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusOK || !strings.Contains(c.Text, keyPrint(key)) {
		t.Fatalf("good key: want OK with fingerprint %s, got %+v\n%s", keyPrint(key), c, r.Text())
	}
	noKeyText(t, r, key)
}

func TestDoctorKeyMissing(t *testing.T) {
	dir, key := keyedConfig(t)
	if err := os.Remove(filepath.Join(dir, secrets.KeyFile)); err != nil {
		t.Fatal(err)
	}
	r := runDoctor(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusFail || !strings.Contains(strings.ToLower(c.Text), "missing") || c.Fix == "" {
		t.Fatalf("missing key: want FAIL saying missing, with a fix, got %+v\n%s", c, r.Text())
	}
	if _, err := os.Stat(filepath.Join(dir, secrets.KeyFile)); err == nil {
		t.Fatal("doctor made a key file")
	}
	_ = key
}

func TestDoctorKeyWrongForDatabase(t *testing.T) {
	dir, key := keyedConfig(t)
	other := make([]byte, secrets.KeySize)
	if err := os.WriteFile(filepath.Join(dir, secrets.KeyFile), []byte(hex.EncodeToString(other)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runDoctor(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusFail || !strings.Contains(strings.ToLower(c.Text), "match") || !strings.Contains(c.Text, keyPrint(other)) {
		t.Fatalf("wrong key: want FAIL saying it does not match, with the file's fingerprint, got %+v\n%s", c, r.Text())
	}
	noKeyText(t, r, other)
	_ = key
}

func TestDoctorKeyUnreadable(t *testing.T) {
	dir, _ := keyedConfig(t)
	p := filepath.Join(dir, secrets.KeyFile)
	if err := os.WriteFile(p, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runDoctor(t, dir)
	if c := worstKey(t, r); c.Status != StatusFail {
		t.Fatalf("damaged key file: want FAIL, got %+v\n%s", c, r.Text())
	}
}

func TestDoctorKeyLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not Unix modes on Windows")
	}
	dir, key := keyedConfig(t)
	if err := os.Chmod(filepath.Join(dir, secrets.KeyFile), 0o644); err != nil {
		t.Fatal(err)
	}
	r := runDoctor(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusWarn || !strings.Contains(c.Fix, "chmod 600") {
		t.Fatalf("loose key file: want WARN with a chmod 600 fix, got %+v\n%s", c, r.Text())
	}
	noKeyText(t, r, key)
}

// HOLYARRS_MASTER_KEY_FILE is read instead of the config folder's file.
func TestDoctorKeyFromEnvFile(t *testing.T) {
	dir, key := keyedConfig(t)
	named := filepath.Join(t.TempDir(), "mk")
	b, err := os.ReadFile(filepath.Join(dir, secrets.KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(named, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, secrets.KeyFile)); err != nil {
		t.Fatal(err)
	}
	t.Setenv(secrets.EnvKeyFile, named)
	r := runDoctorEnv(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusOK || !strings.Contains(c.Text, keyPrint(key)) {
		t.Fatalf("key from %s: want OK with fingerprint, got %+v\n%s", secrets.EnvKeyFile, c, r.Text())
	}
}

// A fresh install (no secrets saved, no key yet) is not a failure: the key
// is made on first start.
func TestDoctorKeyFreshInstall(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	r := runDoctor(t, dir)
	if c := worstKey(t, r); c.Status == StatusFail {
		t.Fatalf("fresh install: key check failed %+v\n%s", c, r.Text())
	}
}

// A key file named in HOLYARRS_MASTER_KEY_FILE that is not there stops the
// start even on a fresh install, so the doctor must not pass it.
func TestDoctorKeyFromEnvFileMissing(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	named := filepath.Join(t.TempDir(), "gone.key")
	t.Setenv(secrets.EnvKeyFile, named)
	r := runDoctorEnv(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusFail || !strings.Contains(c.Text, secrets.EnvKeyFile) || strings.Contains(c.Text, "first start") {
		t.Fatalf("missing named key file: want Fail naming %s, got %+v\n%s", secrets.EnvKeyFile, c, r.Text())
	}
}

// A good key the app's user cannot open is "cannot be read", with the owner
// and the host fix, not "damaged".
func TestDoctorKeyNoPermission(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix modes and a non-root user")
	}
	dir, _ := keyedConfig(t)
	if err := os.Chmod(filepath.Join(dir, secrets.KeyFile), 0); err != nil {
		t.Fatal(err)
	}
	r := runDoctor(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusFail || !strings.Contains(c.Text, "cannot be read") || strings.Contains(c.Text, "damaged") ||
		!strings.Contains(c.Text, "owner") || !strings.Contains(c.Fix, "chown PUID:PGID") || !strings.Contains(c.Fix, "chmod 600") {
		t.Fatalf("unreadable key: got %+v\n%s", c, r.Text())
	}
}

// A key path that is a folder (Docker makes one when the mounted host file
// did not exist) says so, instead of an owner or mode fix that cannot help.
func TestDoctorKeyPathIsFolder(t *testing.T) {
	dir, _ := keyedConfig(t)
	named := filepath.Join(t.TempDir(), "holyarrs.key")
	if err := os.Mkdir(named, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(secrets.EnvKeyFile, named)
	r := runDoctorEnv(t, dir)
	c := worstKey(t, r)
	if c.Status != StatusFail || !strings.Contains(c.Text, "folder") || strings.Contains(c.Fix, "chmod") {
		t.Fatalf("key path is a folder: want FAIL naming the folder without a chmod fix, got %+v\n%s", c, r.Text())
	}
}
