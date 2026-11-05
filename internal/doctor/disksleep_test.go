package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Disks that sleep: a folder that does not answer within the start-up limit
// is most often a disk waking up, not only a slow network share. The words
// say so; the 3 second limit stays.
func TestSlowFolderWordsNameSleepingDisks(t *testing.T) {
	c := (Folder{Role: "Library folder", Path: "/media/movies", State: "slow"}).Check(1, 1)
	if c.Status != Warn {
		t.Fatalf("status %v", c.Status)
	}
	want := "/media/movies: did not answer within 3 seconds, skipped (often a disk waking from sleep or a slow network share); it is checked again later, or run holyarrs doctor"
	if !strings.Contains(c.Text, want) {
		t.Errorf("slow line:\n text %q\n fix  %q\nwant it to say %q", c.Text, c.Fix, want)
	}
	if got := Summary([]Folder{{State: "ok"}, {State: "slow"}}); got != "folders: 1 ok, 1 not checked (slow to answer)" {
		t.Errorf("summary %q", got)
	}
}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// docs/support.md uses the same words as the start-up line.
func TestSupportDocNamesSleepingDisks(t *testing.T) {
	doc := readDoc(t, "support.md")
	for _, want := range []string{"not checked (slow to answer)", "waking from sleep", "3 seconds"} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/support.md does not say %q", want)
		}
	}
}

// docs/nas.md has a "Disks that sleep" section with the advice people on a
// NAS need.
func TestNASDocHasDisksThatSleep(t *testing.T) {
	doc := readDoc(t, "nas.md")
	i := strings.Index(doc, "## Disks that sleep")
	if i < 0 {
		t.Fatal(`docs/nas.md has no "## Disks that sleep" section`)
	}
	sec := doc[i+3:]
	if j := strings.Index(sec, "\n## "); j >= 0 {
		sec = sec[:j]
	}
	low := strings.ToLower(sec)
	for _, want := range []string{"/config", "ssd", "cache pool", "cache directories", "rescan", "7 days", "seeding", "hardlink"} {
		if !strings.Contains(low, want) {
			t.Errorf("Disks that sleep section does not mention %q", want)
		}
	}
}
