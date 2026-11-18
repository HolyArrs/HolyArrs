package updates_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates/signtest"
)

// volume is a config folder with downloaded versions in it.
type volume struct {
	t   *testing.T
	cfg string
	key signtest.Key
	now time.Time
	env map[string]string
	ran []string
}

func newVolume(t *testing.T) *volume {
	cfg := t.TempDir()
	return &volume{t: t, cfg: cfg, key: signtest.New(t), now: time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC), env: map[string]string{}}
}

func (v *volume) app(name string) string { return filepath.Join(v.cfg, updates.AppFolder, name) }

// install puts a signed program for version on the volume and returns its entry.
func (v *volume) install(version string, body string) updates.Installed {
	dir := v.app(version)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		v.t.Fatal(err)
	}
	p := filepath.Join(dir, "holyarrs")
	writeFile(v.t, p, body)
	writeFile(v.t, p+".minisig", string(v.key.Sign([]byte(body), "holyarrs "+version)))
	sum := sha256.Sum256([]byte(body))
	return updates.Installed{Version: version, Path: p, SHA256: hex.EncodeToString(sum[:])}
}

func (v *volume) set(name string, val any) {
	b, _ := json.Marshal(val)
	writeFile(v.t, v.app(name), string(b))
}

func (v *volume) launcher() *updates.Launcher {
	return &updates.Launcher{
		ConfigDir: v.cfg, ImageVersion: "1.1.4", Keys: []updates.PublicKey{v.key.Pub},
		Getenv: func(k string) string { return v.env[k] },
		Exec:   func(p string, _, _ []string) error { v.ran = append(v.ran, p); return errors.New("exec format error") },
		Now:    func() time.Time { return v.now },
	}
}

func (v *volume) status() updates.LaunchStatus {
	var s updates.LaunchStatus
	b, _ := os.ReadFile(v.app("status.json"))
	_ = json.Unmarshal(b, &s)
	return s
}

func (v *volume) exists(name string) bool {
	_, err := os.Stat(v.app(name))
	return err == nil
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchNothingInstalledRunsImage(t *testing.T) {
	v := newVolume(t)
	if _, ok := v.launcher().Pick(); ok {
		t.Fatal("picked a version with nothing installed")
	}
	if v.status().Source != "image" {
		t.Fatalf("source %q", v.status().Source)
	}
}

func TestLaunchPicksGoodVersionAndCountsStart(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "new program"))
	cur, ok := v.launcher().Pick()
	if !ok || cur.Version != "1.2.0" {
		t.Fatalf("picked %v %v", cur, ok)
	}
	if !v.exists("pending.json") || v.status().Source != "update" {
		t.Fatal("no pending record or wrong source")
	}
}

func TestLaunchOffEnvRunsImageAndKeepsFiles(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "new program"))
	v.env["HOLYARRS_UPDATES"] = "off"
	if _, ok := v.launcher().Pick(); ok {
		t.Fatal("off still picked the download")
	}
	if !v.exists("current.json") || !v.status().Off {
		t.Fatal("off should keep the files and say it is off")
	}
}

func TestLaunchImageNewerWinsAndCleans(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.1.3", "old program"))
	if _, ok := v.launcher().Pick(); ok {
		t.Fatal("picked an older download over the image")
	}
	if v.exists("current.json") || v.exists("1.1.3") {
		t.Fatal("old download not removed")
	}
}

func TestLaunchRefusesChangedFileAndUnknownKey(t *testing.T) {
	for name, spoil := range map[string]func(v *volume, in updates.Installed){
		"changed byte": func(_ *volume, in updates.Installed) { writeFile(t, in.Path, "new progrAm") },
		"unknown key": func(_ *volume, in updates.Installed) {
			writeFile(t, in.Path+".minisig", string(signtest.New(t).Sign([]byte("new program"), "x")))
		},
		"missing signature": func(_ *volume, in updates.Installed) { os.Remove(in.Path + ".minisig") },
		"outside its folder": func(v *volume, in updates.Installed) {
			in.Path = filepath.Join(v.cfg, "holyarrs")
			writeFile(t, in.Path, "new program")
			v.set("current.json", in)
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := newVolume(t)
			in := v.install("1.2.0", "new program")
			v.set("current.json", in)
			spoil(v, in)
			if _, ok := v.launcher().Pick(); ok {
				t.Fatal("picked a spoiled download")
			}
			if v.exists("current.json") || !strings.Contains(v.status().Rollback, "1.2.0") {
				t.Fatalf("not rolled back: %+v", v.status())
			}
		})
	}
}

func TestLaunchTwoFailedStartsGoBackToPrevious(t *testing.T) {
	v := newVolume(t)
	prev := v.install("1.2.0", "previous program")
	v.set("previous.json", prev)
	v.set("current.json", v.install("1.3.0", "broken program"))
	l := v.launcher()
	for i := range 2 {
		if cur, ok := l.Pick(); !ok || cur.Version != "1.3.0" {
			t.Fatalf("start %d picked %v", i+1, cur)
		}
	}
	cur, ok := l.Pick()
	if !ok || cur.Version != "1.2.0" {
		t.Fatalf("third start should go back to 1.2.0, got %v %v", cur, ok)
	}
	var skip []string
	b, _ := os.ReadFile(v.app("skip.json"))
	_ = json.Unmarshal(b, &skip)
	if len(skip) != 1 || skip[0] != "1.3.0" {
		t.Fatalf("skip list %v", skip)
	}
}

func TestLaunchMigratingReleaseGoesBackAfterOneFailureAndRestoresCopy(t *testing.T) {
	v := newVolume(t)
	in := v.install("1.2.0", "migrating program")
	in.Migrates = true
	v.set("current.json", in)
	writeFile(t, filepath.Join(v.cfg, database.MainFile), "half-changed")
	writeFile(t, filepath.Join(v.cfg, database.MainFile+"-wal"), "wal")
	writeFile(t, v.app(filepath.Join("dbcopy", database.MainFile)), "before")
	writeFile(t, v.app(filepath.Join("dbcopy", "for")), "1.2.0")
	l := v.launcher()
	if _, ok := l.Pick(); !ok {
		t.Fatal("first start should run")
	}
	if _, ok := l.Pick(); ok {
		t.Fatal("second start of a migrating release should go back")
	}
	b, _ := os.ReadFile(filepath.Join(v.cfg, database.MainFile))
	if string(b) != "before" {
		t.Fatalf("database is %q", b)
	}
	if _, err := os.Stat(filepath.Join(v.cfg, database.MainFile+"-wal")); err == nil {
		t.Fatal("old wal kept beside the restored copy")
	}
}

func TestLaunchOldPendingRecordExpires(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "program"))
	v.set("pending.json", map[string]any{"version": "1.2.0", "starts": 2, "since": v.now.Add(-48 * time.Hour)})
	if _, ok := v.launcher().Pick(); !ok {
		t.Fatal("a two-day-old record should not count")
	}
}

func TestLaunchSkipAndRevoked(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "program"))
	v.set("skip.json", []string{"1.2.0"})
	if _, ok := v.launcher().Pick(); ok {
		t.Fatal("skipped version ran")
	}

	v = newVolume(t)
	v.set("current.json", v.install("1.2.0", "program"))
	list := `{"schema":1,"serial":5,"generated_at":"2026-11-15T00:00:00Z","channels":{},"revoked":["1.2.0"]}`
	writeFile(t, v.app("list.json"), list)
	writeFile(t, v.app("list.json.minisig"), string(v.key.Sign([]byte(list), "serial 5")))
	if _, ok := v.launcher().Pick(); ok {
		t.Fatal("revoked version ran")
	}
	if !strings.Contains(v.status().Rollback, "withdrawn") {
		t.Fatalf("status %+v", v.status())
	}
}

func TestLaunchClearsTempFiles(t *testing.T) {
	v := newVolume(t)
	writeFile(t, v.app(filepath.Join("1.2.0", "holyarrs.tmp")), "half")
	writeFile(t, v.app("current.json.tmp"), "{")
	v.launcher().Pick()
	if v.exists(filepath.Join("1.2.0", "holyarrs.tmp")) || v.exists("current.json.tmp") {
		t.Fatal("temp files left")
	}
}

func TestLaunchExecFailureFallsBackAndSaysWhy(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "program"))
	v.launcher().Launch()
	if len(v.ran) != 1 {
		t.Fatalf("exec calls %v", v.ran)
	}
	s := v.status()
	if s.Source != "image" || !strings.Contains(s.ExecError, "exec format") || v.exists("pending.json") {
		t.Fatalf("status %+v", s)
	}
}

func TestLaunchSkippedInsideVolumeProgram(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "program"))
	v.env["HOLYARRS_FROM_VOLUME"] = "1"
	v.launcher().Launch()
	if len(v.ran) != 0 {
		t.Fatal("the volume program started the launcher again")
	}
}

func TestLaunchRollbackRemovesBadVersionFolder(t *testing.T) {
	v := newVolume(t)
	v.set("current.json", v.install("1.2.0", "program"))
	v.set("skip.json", []string{"1.2.0"})
	v.launcher().Pick()
	if v.exists("1.2.0") {
		t.Fatal("the bad version's folder was kept")
	}
}
