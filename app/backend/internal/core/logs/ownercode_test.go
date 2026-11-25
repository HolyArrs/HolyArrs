package logs

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnerResetCodeStaysOutOfStoredLog(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	const code = "ABCD-EFGH-JKMN-PQRS-TUVW-XYZ2"
	line := "2026/11/19 10:00:00 OWNER RESET CODE: " + code + " (works once)\n"
	var out bytes.Buffer
	_, _ = l.Mask(&out).Write([]byte(line))
	_, _ = l.Write([]byte(line))
	if !strings.Contains(out.String(), code) {
		t.Fatalf("container output lost the code: %q", out.String())
	}
	lines, _ := l.Read(Filter{})
	if len(lines) != 1 || strings.Contains(lines[0].Text, code) || !strings.Contains(lines[0].Text, "OWNER RESET CODE: "+Hidden) {
		t.Fatalf("viewer: %+v", lines)
	}
	if tail := string(l.Tail(1 << 20)); strings.Contains(tail, code) {
		t.Fatalf("download shows the code: %q", tail)
	}
	// Strict: other dashed text is left alone.
	for _, s := range []string{"ABCD-EFGH-JKMN", "abcd-efgh-jkmn-pqrs-tuvw-xyz2", "ABCD-EFGH-JKMN-PQRS-TUVW-XYZ", "ABCD-EFGH-JKMN-PQRS-TUVW-XYZ0"} {
		if HideOwnerCode(s) != s {
			t.Errorf("masked %q", s)
		}
	}
}
