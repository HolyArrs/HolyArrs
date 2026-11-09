package subtitles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// Held subtitle notes may only move files within the Library folder and its
// holding folder, and a restore never overwrites a file.
func TestValidateHeldSubtitles_RejectsEscapes(t *testing.T) {
	lib := t.TempDir()
	movie := filepath.Join(lib, "Arrival (2016)")
	hold := filepath.Join(lib, importer.HoldingDir, "1")
	outside := t.TempDir()
	for _, d := range []string{movie, hold} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	from := filepath.Join(movie, "a.en.srt")
	to := filepath.Join(hold, "a.en.srt")
	cases := []struct {
		name, from, to string
		bad            bool
	}{
		{"valid", from, to, false},
		{"from dot-dot", lib + string(filepath.Separator) + ".." + string(filepath.Separator) + "x.srt", to, true},
		{"from outside", filepath.Join(outside, "x.srt"), to, true},
		{"to in library not holding", from, filepath.Join(movie, "b.srt"), true},
		{"to outside", from, filepath.Join(outside, "x.srt"), true},
		{"to dot-dot out of holding", from, hold + string(filepath.Separator) + ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + "x.srt", true},
	}
	link := filepath.Join(lib, "linked")
	if err := os.Symlink(outside, link); err == nil {
		cases = append(cases, struct {
			name, from, to string
			bad            bool
		}{"from through symlink", filepath.Join(link, "x.srt"), to, true})
	} else {
		t.Logf("no symlink case: %v", err)
	}
	for _, c := range cases {
		err := validateHeldSubtitles(lib, []heldSubtitle{{From: c.from, To: c.to}})
		if (err != nil) != c.bad {
			t.Errorf("%s: err=%v", c.name, err)
		}
		if c.bad {
			if err := restoreHeldSubtitles(lib, []heldSubtitle{{From: c.from, To: c.to}}); err == nil {
				t.Errorf("%s: restore ran", c.name)
			}
		}
	}

	// When both files exist, refuse and leave both alone.
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(from, "new")
	write(to, "old")
	err := restoreHeldSubtitles(lib, []heldSubtitle{{From: from, To: to}})
	if err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(from); string(b) != "new" {
		t.Fatal("from changed")
	}
	if b, _ := os.ReadFile(to); string(b) != "old" {
		t.Fatal("held copy changed")
	}

	// When only the held copy exists, it moves back.
	if err := os.Remove(from); err != nil {
		t.Fatal(err)
	}
	if err := restoreHeldSubtitles(lib, []heldSubtitle{{From: from, To: to}}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(from); string(b) != "old" {
		t.Fatal("not restored")
	}
	if _, err := os.Lstat(to); !os.IsNotExist(err) {
		t.Fatal("held copy left", err)
	}
}
