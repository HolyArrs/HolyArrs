package runas

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadFileEnvTrimsOneNewline(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a", "secret-a\n")
	b := write("b", "secret-b\n\n")
	c := write("c", "secret-c\r\n")
	d := write("d", "secret-d")
	got := map[string]string{}
	set := func(k, v string) error { got[k] = v; return nil }
	env := []string{"FILE__A=" + a, "FILE__B=" + b, "FILE__C=" + c, "FILE__D=" + d, "OTHER=x", "XFILE__E=nope"}
	if err := LoadFileEnv(env, os.ReadFile, set); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "secret-a", "B": "secret-b\n", "C": "secret-c", "D": "secret-d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoadFileEnvMissingFile(t *testing.T) {
	read := func(p string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	set := func(k, v string) error { t.Fatalf("set %s although the file is missing", k); return nil }
	err := LoadFileEnv([]string{"FILE__API_KEY=/run/secrets/nope"}, read, set)
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	msg := err.Error()
	if !strings.Contains(msg, "FILE__API_KEY") || !strings.Contains(msg, "/run/secrets/nope") {
		t.Fatalf("message should name the variable and file: %s", msg)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error should still be a not-exist error: %v", err)
	}
}

func TestLoadFileEnvNeverPrintsValue(t *testing.T) {
	read := func(string) ([]byte, error) { return []byte("hunter2\n"), nil }
	set := func(string, string) error { return errors.New("refused") }
	err := LoadFileEnv([]string{"FILE__PASSWORD=/x"}, read, set)
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error must not contain the value: %v", err)
	}
}

func TestLoadFileEnvNoName(t *testing.T) {
	err := LoadFileEnv([]string{"FILE__=/x"}, os.ReadFile, func(string, string) error { return nil })
	if err == nil {
		t.Fatal("expected an error for FILE__ without a name")
	}
}

func TestParseUmask(t *testing.T) {
	for in, want := range map[string]int{"022": 0o22, "002": 0o2, "0002": 0o2, "0": 0, " 027 ": 0o27, "777": 0o777} {
		got, err := ParseUmask(in)
		if err != nil || got != want {
			t.Errorf("ParseUmask(%q) = %o, %v; want %o", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "8", "098", "1000", "-1", "0x12"} {
		if _, err := ParseUmask(in); err == nil {
			t.Errorf("ParseUmask(%q) should fail", in)
		}
	}
}

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestDecide(t *testing.T) {
	cases := []struct {
		name     string
		euid     int
		env      map[string]string
		switched bool
		want     Plan
		note     string
	}{
		{"root defaults", 0, nil, false, Plan{Action: Switch, UID: 1000, GID: 1000, Umask: -1}, ""},
		{"root 99:100 umask", 0, map[string]string{"PUID": "99", "PGID": "100", "UMASK": "002"}, false,
			Plan{Action: Switch, UID: 99, GID: 100, Umask: 0o2}, ""},
		{"root PUID 0 PGID 0", 0, map[string]string{"PUID": "0", "PGID": "0"}, false,
			Plan{Action: StayRoot, UID: 0, GID: 0, Umask: -1}, "PUID=0"},
		{"root empty PUID is default", 0, map[string]string{"PUID": "", "PGID": " "}, false,
			Plan{Action: Switch, UID: 1000, GID: 1000, Umask: -1}, ""},
		{"non-root no PUID", 1000, nil, false, Plan{Action: Keep, UID: 1000, GID: 1000, Umask: -1}, ""},
		{"non-root with PUID", 1001, map[string]string{"PUID": "99"}, false,
			Plan{Action: Keep, UID: 99, GID: 1000, Umask: -1}, "ignored: the container already runs as 1001:1001"},
		{"non-root with PGID only", 1001, map[string]string{"PGID": "100", "UMASK": "022"}, false,
			Plan{Action: Keep, UID: 1000, GID: 100, Umask: 0o22}, "ignored"},
		{"after switch, quiet", 99, map[string]string{"PUID": "99", "PGID": "100"}, true,
			Plan{Action: Keep, UID: 99, GID: 100, Umask: -1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decide(c.euid, c.euid, envOf(c.env), c.switched)
			if err != nil {
				t.Fatal(err)
			}
			note := got.Note
			got.Note = ""
			if got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			if c.note == "" && note != "" || !strings.Contains(note, c.note) {
				t.Fatalf("note %q, want it to contain %q", note, c.note)
			}
		})
	}
}

func TestDecideErrors(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"bad PUID":  {"PUID": "abc"},
		"neg PGID":  {"PGID": "-5"},
		"bad UMASK": {"UMASK": "999"},
	} {
		if _, err := Decide(0, 0, envOf(env), false); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// Still root after the switch: never carry on as root.
	if _, err := Decide(0, 0, envOf(nil), true); err == nil || !strings.Contains(err.Error(), "will not run as root") {
		t.Fatalf("root after switch should stop: %v", err)
	}
}

// The egid differs from the euid here, so a GID copied from the current group
// would show.
func TestDecidePUIDZero(t *testing.T) {
	got, err := Decide(0, 5, envOf(map[string]string{"PUID": "0", "PGID": "0"}), false)
	if err != nil || got.Action != StayRoot || got.UID != 0 || got.GID != 0 || !strings.Contains(got.Note, "PGID=0") {
		t.Fatalf("PUID=0 PGID=0: got %+v, %v", got, err)
	}
	for _, env := range []map[string]string{{"PUID": "0"}, {"PUID": "0", "PGID": "100"}, {"PUID": "0", "PGID": " "}} {
		if _, err := Decide(0, 5, envOf(env), false); err == nil || !strings.Contains(err.Error(), "PUID=0 needs PGID=0") {
			t.Errorf("%v: want the PUID=0 needs PGID=0 error, got %v", env, err)
		}
	}
	// PGID=0 with a normal PUID is an ordinary switch.
	got, err = Decide(0, 5, envOf(map[string]string{"PUID": "1000", "PGID": "0"}), false)
	if err != nil || got.Action != Switch || got.UID != 1000 || got.GID != 0 {
		t.Fatalf("PUID=1000 PGID=0: got %+v, %v", got, err)
	}
}

func TestPerm(t *testing.T) {
	old := startMask
	defer func() { startMask = old }()
	startMask = 0o002
	if got := Perm(0o666); got != 0o664 {
		t.Fatalf("Perm(0666) with umask 002 = %o, want 664", got)
	}
	startMask = 0o022
	if got := Perm(0o666); got != 0o644 {
		t.Fatalf("Perm(0666) with umask 022 = %o, want 644", got)
	}
}

func TestNeedsOwnerFix(t *testing.T) {
	if NeedsOwnerFix(1000, 1000, 1000, 1000) {
		t.Fatal("same owner needs no pass")
	}
	if !NeedsOwnerFix(1000, 1000, 99, 100) || !NeedsOwnerFix(0, 0, 1000, 1000) || !NeedsOwnerFix(1000, 0, 1000, 1000) {
		t.Fatal("different owner needs a pass")
	}
}

func TestGroups(t *testing.T) {
	cases := []struct {
		gid  int
		cur  []int
		want []int
	}{
		{100, nil, []int{100}},
		{100, []int{0}, []int{100}},
		{100, []int{0, 44, 100, 109, 44}, []int{100, 44, 109}},
		{0, []int{0, 44}, []int{0, 44}},
	}
	for _, c := range cases {
		if got := Groups(c.gid, c.cur); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Groups(%d, %v) = %v, want %v", c.gid, c.cur, got, c.want)
		}
	}
}

func TestSwitchFailedMessage(t *testing.T) {
	msg := SwitchFailed(99, 100, errors.New("operation not permitted")).Error()
	for _, want := range []string{"99:100", "operation not permitted", "SETUID", "SETGID", "CHOWN", `user: "99:100"`, "will not run as root"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %s", want, msg)
		}
	}
}
