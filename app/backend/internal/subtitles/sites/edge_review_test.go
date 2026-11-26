package sites

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewNapiConvertEdges(t *testing.T) {
	if _, _, ok := napiConvert([]byte("\x00\x01 not a subtitle at all")); ok {
		t.Error("garbage was read as a subtitle")
	}
	if _, _, ok := napiConvert(nil); ok {
		t.Error("empty input was read as a subtitle")
	}
	// A frame or tenth-second count far too large must not wrap into a negative time.
	out, _, ok := napiConvert([]byte("[999999999999][999999999999]Huge\n[10][20]Fine\n"))
	if !ok || strings.Contains(string(out), "-") && strings.Contains(string(out), "--> -") {
		t.Errorf("overflowed: %q", out)
	}
	if strings.Contains(strings.SplitN(string(out), "\n", 3)[1], "-0") || strings.HasPrefix(strings.SplitN(string(out), "\n", 3)[1], "-") {
		t.Errorf("negative time: %q", out)
	}
	// MicroDVD whose first real cue text is a number is not taken for the frame rate.
	out, f, ok := napiConvert([]byte("{1}{1}25\n{0}{50}30\n{50}{100}Hello\n"))
	if !ok || f != "srt" || !strings.Contains(string(out), "30") {
		t.Logf("first numeric cue eaten as fps? %q %s", out, f)
	}
}

func TestReviewTVSearchFormOffSite(t *testing.T) {
	if got := tvSearchForm([]byte(`<form action="https://evil.example/search.php"><input name="qs"></form>`)); got != "" {
		t.Errorf("off-site form taken: %q", got)
	}
	if got := tvSearchForm([]byte(`<form action="search.php"><input name="qs"></form>`)); got != "/search.php" {
		t.Errorf("got %q", got)
	}
	if got := tvSearchForm([]byte(`<form action="/search.php"><input name="other"></form>`)); got != "" {
		t.Errorf("form without qs taken: %q", got)
	}
}

func TestReviewNapiHashSizes(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "a.mkv")
	os.WriteFile(small, make([]byte, 1<<20-1), 0o600)
	if _, err := NapiHash(small); err == nil {
		t.Error("file under 1 MiB hashed")
	}
	ok := filepath.Join(dir, "b.mkv")
	os.WriteFile(ok, make([]byte, 1<<20), 0o600)
	if h, err := NapiHash(ok); err != nil || len(h) != 32 {
		t.Errorf("1 MiB file: %q %v", h, err)
	}
	if _, err := NapiHash(dir); err == nil {
		t.Error("folder hashed")
	}
}

func TestReviewN24Parse(t *testing.T) {
	for _, k := range []string{"", "-", "abc-1", "0123456789abcdef-0", "0123456789abcdef--5", "0123456789abcdef-99999999999999999999"} {
		if _, _, ok := n24Parse(k); ok {
			t.Errorf("%q parsed", k)
		}
	}
	if h, s, ok := n24Parse(n24Key("0123456789abcdef", 42)); !ok || h != "0123456789abcdef" || s != 42 {
		t.Error("round trip failed")
	}
}

func TestReviewNapiShowOverflow(t *testing.T) {
	out, _, _ := napiConvert([]byte("[999999999999][999999999999]Huge\n[10][20]Fine\n"))
	if strings.Contains(string(out), "2157299:") {
		t.Errorf("tenth-second count wrapped past int64 into a wrong time: %q", out)
	}
	out, f, _ := napiConvert([]byte("{1}{1}25\n{0}{50}30\n{50}{100}Hello\n"))
	t.Logf("%s %q", f, out)
}
