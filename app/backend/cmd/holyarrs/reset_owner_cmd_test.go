package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestResetOwnerCommand(t *testing.T) {
	val := "a long phrase" // sample value, not a real login
	cases := []struct {
		label, in string
		tty       bool
		pws       []string
		code      int
		says      string
	}{
		{label: "docker exec -i", in: "luis\n" + val + "\n", code: 0, says: `Done. Sign in as "luis"`},
		{label: "no -i: empty stdin", in: "", code: 1, says: "docker exec -it holyarrs /holyarrs reset-owner"},
		{label: "short password", in: "luis\nabc\n", code: 1, says: "Nothing was changed"},
		{label: "terminal", in: "luis\n", tty: true, pws: []string{val, val}, code: 0, says: "Done."},
		{label: "terminal retry", in: "luis\nluis\n", tty: true, pws: []string{val, "other phrase!", val, val}, code: 0, says: "differ. Try again"},
		{label: "terminal gives up", in: "luis\nluis\nluis\n", tty: true, pws: []string{"x", "y", "z"}, code: 1, says: "Nothing was changed"},
		{label: "terminal Ctrl+D at password", in: "luis\n", tty: true, code: 1, says: "no password came in. Nothing was changed"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		var out bytes.Buffer
		pws := c.pws
		p := ownerPrompt{in: strings.NewReader(c.in), out: &out, tty: c.tty, wait: time.Second, readPw: func() (string, error) {
			if len(pws) == 0 {
				return "", io.EOF
			}
			s := pws[0]
			pws = pws[1:]
			return s, nil
		}}
		if code := p.run([]string{"-config", dir}); code != c.code || !strings.Contains(out.String(), c.says) {
			t.Errorf("%s: code %d, output:\n%s", c.label, code, out.String())
		}
		if strings.Contains(out.String(), val) {
			t.Errorf("%s: the password was printed", c.label)
		}
	}
}

func TestReadLineWithinTimesOut(t *testing.T) {
	// docker exec -t without -i: a terminal that never sends typing.
	r, _ := io.Pipe()
	if _, err := readLineWithin(bufio.NewReader(r), 20*time.Millisecond); !errors.Is(err, errNoTyping) {
		t.Fatalf("got %v", err)
	}
}

func TestResetOwnerCodePrintsAFreshCode(t *testing.T) {
	dir := t.TempDir()
	var out, code bytes.Buffer
	p := ownerPrompt{in: strings.NewReader(""), out: &out, code: &code}
	// No owner yet: no code.
	if rc := p.run([]string{"-config", dir, "--code"}); rc != 1 {
		t.Fatalf("no owner: code %d, output:\n%s", rc, out.String())
	}
	p.in = strings.NewReader("luis\na long phrase\n")
	if rc := p.run([]string{"-config", dir}); rc != 0 {
		t.Fatalf("reset: %s", out.String())
	}
	p.in = strings.NewReader("")
	if rc := p.run([]string{"-config", dir, "--code"}); rc != 0 || !strings.Contains(out.String(), "Forgot password?") {
		t.Fatalf("--code: %d %s", rc, out.String())
	}
	got := strings.TrimSpace(code.String())
	if len(got) != 29 || strings.Count(got, "-") != 5 {
		t.Fatalf("code %q", got)
	}
}
