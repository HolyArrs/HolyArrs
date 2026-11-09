package startup

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
		want     RunAsPlan
		note     string
	}{
		{"root defaults", 0, nil, false, RunAsPlan{Action: Switch, UID: 1000, GID: 1000, Umask: -1}, ""},
		{"root 99:100 umask", 0, map[string]string{"PUID": "99", "PGID": "100", "UMASK": "002"}, false,
			RunAsPlan{Action: Switch, UID: 99, GID: 100, Umask: 0o2}, ""},
		{"root PUID 0 PGID 0", 0, map[string]string{"PUID": "0", "PGID": "0"}, false,
			RunAsPlan{Action: StayRoot, UID: 0, GID: 0, Umask: -1}, "PUID=0"},
		{"root empty PUID is default", 0, map[string]string{"PUID": "", "PGID": " "}, false,
			RunAsPlan{Action: Switch, UID: 1000, GID: 1000, Umask: -1}, ""},
		{"non-root no PUID", 1000, nil, false, RunAsPlan{Action: Keep, UID: 1000, GID: 1000, Umask: -1}, ""},
		{"non-root with PUID", 1001, map[string]string{"PUID": "99"}, false,
			RunAsPlan{Action: Keep, UID: 99, GID: 1000, Umask: -1}, "ignored: the container already runs as 1001:1001"},
		{"non-root with PGID only", 1001, map[string]string{"PGID": "100", "UMASK": "022"}, false,
			RunAsPlan{Action: Keep, UID: 1000, GID: 100, Umask: 0o22}, "ignored"},
		{"after switch, quiet", 99, map[string]string{"PUID": "99", "PGID": "100"}, true,
			RunAsPlan{Action: Keep, UID: 99, GID: 100, Umask: -1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DecideRunAs(c.euid, c.euid, envOf(c.env), c.switched, true)
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
		if _, err := DecideRunAs(0, 0, envOf(env), false, true); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// Still root after the switch: never carry on as root.
	if _, err := DecideRunAs(0, 0, envOf(nil), true, true); err == nil || !strings.Contains(err.Error(), "will not run as root") {
		t.Fatalf("root after switch should stop: %v", err)
	}
}

// The egid differs from the euid here, so a GID copied from the current group
// would show.
func TestDecidePUIDZero(t *testing.T) {
	got, err := DecideRunAs(0, 5, envOf(map[string]string{"PUID": "0", "PGID": "0"}), false, true)
	if err != nil || got.Action != StayRoot || got.UID != 0 || got.GID != 0 || !strings.Contains(got.Note, "PGID=0") {
		t.Fatalf("PUID=0 PGID=0: got %+v, %v", got, err)
	}
	for _, env := range []map[string]string{{"PUID": "0"}, {"PUID": "0", "PGID": "100"}, {"PUID": "0", "PGID": " "}} {
		if _, err := DecideRunAs(0, 5, envOf(env), false, true); err == nil || !strings.Contains(err.Error(), "PUID=0 needs PGID=0") {
			t.Errorf("%v: want the PUID=0 needs PGID=0 error, got %v", env, err)
		}
	}
	// PGID=0 with a normal PUID is an ordinary switch.
	got, err = DecideRunAs(0, 5, envOf(map[string]string{"PUID": "1000", "PGID": "0"}), false, true)
	if err != nil || got.Action != Switch || got.UID != 1000 || got.GID != 0 {
		t.Fatalf("PUID=1000 PGID=0: got %+v, %v", got, err)
	}
}

func TestDecideOutsideContainer(t *testing.T) {
	// Root without PUID/PGID: stop, never switch to the 1000:1000 default.
	for _, env := range []map[string]string{nil, {"PUID": "", "PGID": " "}, {"UMASK": "002"}} {
		got, err := DecideRunAs(0, 0, envOf(env), false, false)
		if err == nil || err.Error() != NotInContainer || got.Action == Switch {
			t.Errorf("%v: want the outside-a-container error, got %+v, %v", env, got, err)
		}
	}
	// PUID or PGID set by hand: switching is fine.
	for _, env := range []map[string]string{{"PUID": "1001", "PGID": "1001"}, {"PUID": "1001"}, {"PGID": "1001"}} {
		got, err := DecideRunAs(0, 0, envOf(env), false, false)
		if err != nil || got.Action != Switch {
			t.Errorf("%v: want a switch, got %+v, %v", env, got, err)
		}
	}
	// PUID=0 PGID=0 still means root, and a non-root start keeps its user.
	if got, err := DecideRunAs(0, 0, envOf(map[string]string{"PUID": "0", "PGID": "0"}), false, false); err != nil || got.Action != StayRoot {
		t.Errorf("PUID=0 PGID=0 outside a container: got %+v, %v", got, err)
	}
	if got, err := DecideRunAs(1000, 1000, envOf(nil), false, false); err != nil || got.Action != Keep {
		t.Errorf("non-root outside a container: got %+v, %v", got, err)
	}
}

func TestInContainer(t *testing.T) {
	files := func(have ...string) func(string) bool {
		return func(p string) bool {
			for _, h := range have {
				if h == p {
					return true
				}
			}
			return false
		}
	}
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "container" {
				return v
			}
			return ""
		}
	}
	cases := []struct {
		name   string
		exists func(string) bool
		getenv func(string) string
		want   bool
	}{
		{"docker", files("/.dockerenv"), env(""), true},
		{"podman file", files("/run/.containerenv"), env(""), true},
		{"container variable", files(), env("podman"), true},
		{"plain host", files("/etc/passwd"), env(""), false},
		{"kubernetes", files(), func(k string) string {
			if k == "KUBERNETES_SERVICE_HOST" {
				return "10.43.0.1"
			}
			return ""
		}, true},
	}
	for _, c := range cases {
		if got := InContainer(c.exists, c.getenv); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
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
	for _, want := range []string{"99:100", "operation not permitted", "CHOWN", "DAC_OVERRIDE", "SETUID", "SETGID", `user: "99:100"`, "will not run as root"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %s", want, msg)
		}
	}
}

func TestMountsUnder(t *testing.T) {
	info := strings.Join([]string{
		"22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw",
		"30 22 8:1 /var/lib/docker/config /config rw,relatime - ext4 /dev/sda1 rw",
		"31 30 8:1 /srv/media /config/media rw,relatime - ext4 /dev/sda1 rw",
		`32 30 8:1 /srv/my\040films /config/my\040films rw - ext4 /dev/sda1 rw`,
		"33 30 0:50 / /config/cache/tmp rw - tmpfs tmpfs rw",
		"34 30 8:1 /etc/hosts.txt /config/hosts.txt ro - ext4 /dev/sda1 rw",
		"35 22 8:1 /srv/configx /configx rw - ext4 /dev/sda1 rw",
		"36 22 8:1 /srv/downloads /downloads rw - ext4 /dev/sda1 rw",
		"",
		"short line",
	}, "\n")
	got := MountsUnder(info, "/config/")
	want := map[string]bool{"media": true, "my films": true, "cache/tmp": true, "hosts.txt": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MountsUnder = %v, want %v", got, want)
	}
	if got := MountsUnder(info, "/srv"); len(got) != 0 {
		t.Fatalf("no mounts under /srv, got %v", got)
	}
	if got := MountsUnder(info, "/"); !got["config"] || !got["config/media"] || got[""] {
		t.Fatalf("mounts under / wrong: %v", got)
	}
}

func TestUnescapeMount(t *testing.T) {
	for in, want := range map[string]string{
		`/a\040b`:  "/a b",
		`/t\011ab`: "/t\tab",
		`/b\134s`:  `/b\s`,
		`/plain`:   "/plain",
		`/odd\04`:  `/odd\04`,
		`/bad\9xy`: `/bad\9xy`,
	} {
		if got := unescapeMount(in); got != want {
			t.Errorf("unescapeMount(%q) = %q, want %q", in, got, want)
		}
	}
}
