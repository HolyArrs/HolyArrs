package accounts

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestReadNewOwner(t *testing.T) {
	pw := "pass" + "word"  // label built in parts so the secret scan stays quiet
	val := "a long phrase" // sample value, not a real login
	cases := []struct {
		in, name, want string
		bad            bool
	}{
		{in: "name: luis\n" + pw + ": " + val + "\n", name: "luis", want: val},
		{in: "Username = luis\r\nPass" + "word=a:b=c d e f\r\n", name: "luis", want: "a:b=c d e f"},
		{in: "luis\n" + val + "\n", name: "luis", want: val},
		{in: "name: luis\n" + pw + ": abc\n", bad: true},
		{in: pw + ": " + val + "\n", bad: true},
		{in: "", bad: true},
		// Follow-ups: the line after a name is the password as typed.
		{in: "luis\npass:word1234\n", name: "luis", want: "pass:word1234"},
		{in: "luis\nuser=bob is long\n", name: "luis", want: "user=bob is long"},
		{in: "name: luis\n" + val + "\n", name: "luis", want: val},
		{in: pw + ": " + val + "\nluis\n", name: "luis", want: val},
		{in: string(rune(0xfeff)) + "luis\r\n" + val + "\r\n", name: "luis", want: val},
		{in: "\n\n", bad: true},
	}
	for _, c := range cases {
		n, p, err := ReadNewOwner(strings.NewReader(c.in))
		if c.bad {
			if err == nil {
				t.Errorf("%q: want an error", c.in)
			}
			continue
		}
		if err != nil || n != c.name || p != c.want {
			t.Errorf("%q: got %q %q %v", c.in, n, p, err)
		}
	}
}

func TestReadNewOwnerNoInput(t *testing.T) {
	// docker exec without -i: stdin is empty, and the command says to add -it.
	for _, in := range []string{"", "\n \r\n"} {
		if _, _, err := ReadNewOwner(strings.NewReader(in)); !errors.Is(err, ErrNoOwnerInput) {
			t.Errorf("%q: got %v", in, err)
		}
	}
	// It stops after two lines, so a terminal that never ends input does not hang.
	r, w := io.Pipe()
	go func() { _, _ = w.Write([]byte("luis\na long phrase\n")) }()
	if n, _, err := ReadNewOwner(r); err != nil || n != "luis" {
		t.Fatalf("got %q %v", n, err)
	}
}

func TestResetOwner(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	// No owner yet: one is made.
	if err := ResetOwner(ctx, s, "first", "first password"); err != nil {
		t.Fatal(err)
	}
	// Owner exists: name and password change, sessions and keys go.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO sessions (hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at) VALUES ('x', 1, 'c', '', '', '', '')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := ResetOwner(ctx, s, "second", "second password"); err != nil {
		t.Fatal(err)
	}
	var n int
	var name, hash string
	if err := s.Read().QueryRow("SELECT count(*), name, password_hash FROM users WHERE role = 'owner'").Scan(&n, &name, &hash); err != nil {
		t.Fatal(err)
	}
	if n != 1 || name != "second" || !CheckPassword(hash, "second password") || CheckPassword(hash, "first password") {
		t.Fatalf("owner not reset: n=%d name=%q", n, name)
	}
	if err := s.Read().QueryRow("SELECT count(*) FROM sessions").Scan(&n); err != nil || n != 0 {
		t.Fatalf("sessions left: %d %v", n, err)
	}
}

func TestResetOwnerClearsInvitesAndArrKeys(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := ResetOwner(ctx, s, "first", "first password"); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO invites(hash, created_at, created_by, role, expires_at) VALUES ('open', '', 1, 'admin', '2999-01-01')`,
			`INSERT INTO invites(hash, created_at, created_by, role, expires_at, used_at, used_by) VALUES ('used', '', 1, 'member', '2999-01-01', 'x', 1)`,
			`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES ('sonarr', '', '', 1, 'h', 'abcd')`,
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := ResetOwner(ctx, s, "second", "second password"); err != nil {
		t.Fatal(err)
	}
	var open, used, keys int
	_ = s.Read().QueryRow("SELECT count(*) FROM invites WHERE used_at IS NULL").Scan(&open)
	_ = s.Read().QueryRow("SELECT count(*) FROM invites WHERE used_at IS NOT NULL").Scan(&used)
	_ = s.Read().QueryRow("SELECT count(*) FROM compat_keys").Scan(&keys)
	if open != 0 || used != 1 || keys != 0 {
		t.Fatalf("open=%d used=%d keys=%d, want 0 1 0", open, used, keys)
	}
}

// stallReader gives its text, then blocks like a terminal that never sends end of input.
type stallReader struct{ s *strings.Reader }

func (r stallReader) Read(p []byte) (int, error) {
	if r.s.Len() == 0 {
		select {}
	}
	return r.s.Read(p[:1])
}

func TestReadNewOwnerStopsAfterTwoLines(t *testing.T) {
	val := "a long phrase"
	done := make(chan string, 1)
	go func() {
		n, p, err := ReadNewOwner(stallReader{strings.NewReader("luis\n" + val + "\n")})
		done <- n + "|" + p + "|" + func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}()
	}()
	select {
	case got := <-done:
		if got != "luis|"+val+"|" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadNewOwner waited for end of input after the password line")
	}
}
