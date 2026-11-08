package health

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Disks that sleep: a folder that does not answer within the time limit
// is most often a disk waking up, not only a slow network share. The line
// names the limit of the run that skipped it (3 seconds at start-up, 20 in
// holyarrs doctor), and only the start-up line sends people to the doctor.
func TestSlowFolderWordsNameSleepingDisks(t *testing.T) {
	c := (Folder{Role: "Library folder", Path: "/media/movies", State: "slow", Budget: 3 * time.Second}).Check(1, 1)
	if c.Status != StatusWarn {
		t.Fatalf("status %v", c.Status)
	}
	want := "Library folder /media/movies: did not answer within 3 seconds and was skipped (often a disk waking from sleep or a slow network share)"
	if c.Text != want {
		t.Errorf("start-up slow line: text %q, want %q", c.Text, want)
	}
	if c.Fix != "run holyarrs doctor to check it without the start-up time limit" {
		t.Errorf("start-up fix %q", c.Fix)
	}
	if got := Summary([]Folder{{State: "ok"}, {State: "slow"}}); got != "folders: 1 ok, 1 not checked (slow to answer)" {
		t.Errorf("summary %q", got)
	}
}

// Inside holyarrs doctor (20 second limit) the line says 20 seconds and does
// not tell people to run the command they are already running.
func TestSlowFolderInDoctorRun(t *testing.T) {
	c := (Folder{Role: "downloads", Path: "/downloads", State: "slow", Budget: 20 * time.Second}).Check(1, 1)
	if !strings.Contains(c.Text, "did not answer within 20 seconds") {
		t.Errorf("doctor slow text %q", c.Text)
	}
	if strings.Contains(c.Text, "holyarrs doctor") {
		t.Errorf("doctor slow text names the doctor: %q", c.Text)
	}
	if c.Fix != "wake the disk or check the share, then run holyarrs doctor again" {
		t.Errorf("doctor slow fix %q", c.Fix)
	}
	// No budget known: plain words, no number.
	c = (Folder{Role: "downloads", Path: "/d", State: "slow"}).Check(1, 1)
	if !strings.Contains(c.Text, "did not answer in time and was skipped") {
		t.Errorf("slow text without budget %q", c.Text)
	}
}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// docs/help/troubleshooting.md uses the same words as the start-up line.
func TestSupportDocNamesSleepingDisks(t *testing.T) {
	doc := readDoc(t, filepath.Join("help", "troubleshooting.md"))
	for _, want := range []string{"not checked", "waking from sleep", "3 seconds"} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/help/troubleshooting.md does not say %q", want)
		}
	}
	// Download folders are not checked on a timer, so no promise of a later
	// check.
	if strings.Contains(doc, "checked again later") {
		t.Error("docs/help/troubleshooting.md still says the folder is checked again later")
	}
}

// docs/run/platforms.md has a "Disks that sleep" section with the advice people on a
// NAS need.
func TestNASDocHasDisksThatSleep(t *testing.T) {
	doc := readDoc(t, filepath.Join("run", "platforms.md"))
	i := strings.Index(doc, "### Let your disks sleep")
	if i < 0 {
		t.Fatal(`docs/run/platforms.md has no "### Let your disks sleep" section`)
	}
	sec := doc[i+3:]
	if j := strings.Index(sec, "\n## "); j >= 0 {
		sec = sec[:j]
	}
	low := strings.ToLower(sec)
	// The app has no button called Rescan; the doc names the real one.
	if strings.Contains(sec, "press Rescan") || !strings.Contains(sec, "Run now") {
		t.Error(`Disks that sleep section should send people to "Run now" on Rescan the Library, not a Rescan button`)
	}
	for _, want := range []string{"/config", "ssd", "cache pool", "cache directories", "rescan", "7 days", "seeding", "hardlink"} {
		if !strings.Contains(low, want) {
			t.Errorf("Disks that sleep section does not mention %q", want)
		}
	}
}
