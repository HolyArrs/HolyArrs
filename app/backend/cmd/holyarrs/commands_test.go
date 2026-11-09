package main

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// withStdin makes text the program's standard input for the test.
func withStdin(t *testing.T, text string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(text); err != nil {
		t.Fatal(err)
	}
	w.Close()
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev; r.Close() })
}

func TestResetOwnerCmd(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.ResetOwner(ctx, s, "luis", "old password here"); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO sessions (hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at) VALUES ('x', 1, 'c', '', '', '', '')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	logs := captureLog(t)
	withStdin(t, "name: maya\npassword: newpass99\n")
	if rc := resetOwner([]string{"-config", dir}); rc != 0 {
		t.Fatalf("exit %d: %s", rc, logs)
	}
	if strings.Contains(logs.String(), "newpass99") {
		t.Fatal("the new password was logged")
	}

	s, err = database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var name, hash string
	var n int
	if err := s.Read().QueryRow("SELECT count(*), name, password_hash FROM users WHERE role = 'owner'").Scan(&n, &name, &hash); err != nil {
		t.Fatal(err)
	}
	if n != 1 || name != "maya" || !accounts.CheckPassword(hash, "newpass99") || accounts.CheckPassword(hash, "old password here") {
		t.Fatalf("owner not reset: n=%d name=%q", n, name)
	}
	if err := s.Read().QueryRow("SELECT count(*) FROM sessions").Scan(&n); err != nil || n != 0 {
		t.Fatalf("sessions left: %d %v", n, err)
	}
}

func TestResetOwnerCmdRefusesBadInput(t *testing.T) {
	dir := t.TempDir()
	captureLog(t)
	withStdin(t, "")
	if rc := resetOwner([]string{"-config", dir}); rc != 1 {
		t.Fatalf("empty stdin: exit %d, want 1", rc)
	}
	withStdin(t, "name: maya\npassword: short\n")
	if rc := resetOwner([]string{"-config", dir}); rc != 1 {
		t.Fatalf("short password: exit %d, want 1", rc)
	}
	withStdin(t, "name: maya\npassword: newpass99\n")
	if rc := resetOwner([]string{"-config", dir, "extra"}); rc != 2 {
		t.Fatalf("bad flags: exit %d, want 2", rc)
	}
}

func TestLogFolderCheckWarnsUnsafeFolder(t *testing.T) {
	configDir := t.TempDir()
	s, err := database.Open(configDir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO library_folders (created_at, created_by, updated_at, updated_by, path, kind, label, health, same_disk_as) VALUES ('', '{}', '', '{}', ?, 'movie', 'Movies', 'ok', '[]')`, configDir)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	logs := captureLog(t)
	logFolderCheck(ctx, s.Read(), t.TempDir(), configDir)
	if out := logs.String(); !strings.Contains(out, "WARN") || !strings.Contains(out, "overlaps the config folder") {
		t.Fatalf("no unsafe-folder warning:\n%s", out)
	}

	// A different config folder leaves the Library folder alone.
	logs = captureLog(t)
	logFolderCheck(ctx, s.Read(), t.TempDir(), t.TempDir())
	if strings.Contains(logs.String(), "overlaps the config folder") {
		t.Fatalf("warned about a folder that is safe:\n%s", logs)
	}
}

func TestEnvOn(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "true": true, "TRUE": true, "on": true, "yes": true, " On ": true, "0": false, "": false, "off": false, "no": false, "2": false} {
		t.Setenv("HOLYARRS_TEST_ENVON", v)
		if got := envOn("HOLYARRS_TEST_ENVON"); got != want {
			t.Errorf("envOn(%q) = %v, want %v", v, got, want)
		}
	}
}
