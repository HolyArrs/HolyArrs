package youtube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		`a/b\c:d*e?f"g<h>i|j`: "a_b_c_d_e_f_g_h_i_j",
		"..":                  "untitled",
		".":                   "untitled",
		"":                    "untitled",
		"  x  y. ":            "x y",
		"tab\there\nnew\x00z": "tab_here_new_z",
		"Zażółć gęślą jaźń 🎬": "Zażółć gęślą jaźń 🎬",
		"...hidden...":        "hidden",
		"100% %(id)s":         "100% %(id)s",
	}
	for in, want := range cases {
		if got := SafeName(in, 120); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("ż", 150) // 300 bytes
	got := SafeName(long, 120)
	if len(got) > 120 || !utf8.ValidString(got) || got != strings.Repeat("ż", 60) {
		t.Errorf("long title cut to %d bytes %q", len(got), got)
	}
	if got := SafeName("ab"+strings.Repeat("🎬", 3), 7); got != "ab🎬" {
		t.Errorf("cut on a rune boundary gave %q", got)
	}
	if got := SafeName("abc .x", 5); got != "abc" {
		t.Errorf("cut then trim gave %q", got)
	}
}

func TestVideoBaseEscapesTemplate(t *testing.T) {
	day := time.Date(2024, 3, 5, 23, 30, 0, 0, time.UTC)
	base := VideoBase(&day, "100% %(id)s", "abcdefghijk")
	if base != "2024-03-05 100% %(id)s [abcdefghijk]" {
		t.Fatalf("VideoBase = %q", base)
	}
	args := downloadArgs(Tools{}, "abcdefghijk", 1080, "/lib/7-Anna/Fun/"+base)
	if !containsArg(args, "/lib/7-Anna/Fun/2024-03-05 100%% %%(id)s [abcdefghijk].%(ext)s") {
		t.Fatalf("-o argument not escaped: %q", args)
	}
	if got := VideoBase(nil, "a/b", "abcdefghijk"); got != "0000-00-00 a_b [abcdefghijk]" {
		t.Fatalf("VideoBase(nil) = %q", got)
	}
	if got := VideoBase(nil, "", "abcdefghijk"); got != "0000-00-00 untitled [abcdefghijk]" {
		t.Fatalf("VideoBase(empty title) = %q", got)
	}
	if got := VideoBase(nil, strings.Repeat("x", 300), "abcdefghijk"); got != "0000-00-00 "+strings.Repeat("x", 120)+" [abcdefghijk]" {
		t.Fatalf("VideoBase(long) = %q", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestUserDirReusesIDPrefix(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"7-Anna", "70-Bob", "17-Cid"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "8-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := UserDir(root, 7, "Ania")
	if err != nil || got != filepath.Join(root, "7-Anna") {
		t.Fatalf("UserDir(7) = %q, %v", got, err)
	}
	got, err = UserDir(root, 9, "Ania")
	if err != nil || got != filepath.Join(root, "9-Ania") {
		t.Fatalf("UserDir(9) = %q, %v", got, err)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("UserDir did not create %q: %v", got, err)
	}
	got, err = UserDir(root, 8, "a/b")
	if err != nil || got != filepath.Join(root, "8-a_b") {
		t.Fatalf("UserDir(8, a/b) = %q, %v (a file named 8-file is not a folder)", got, err)
	}
	got, err = UserDir(filepath.Join(root, "new", "lib"), 3, strings.Repeat("n", 100))
	if err != nil || got != filepath.Join(root, "new", "lib", "3-"+strings.Repeat("n", 60)) {
		t.Fatalf("UserDir(missing root) = %q, %v", got, err)
	}
}
