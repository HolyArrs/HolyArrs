package health

import (
	"encoding/json"
	"strings"
	"testing"
)

// Disks that sleep: the daily Library rescan can be turned off like the
// movie details refresh; other schedules still cannot.
func TestRescanScheduleMayBeTurnedOff(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	b := do(t, "PUT", f.srv.URL+"/api/v1/schedules/rescan_library", `{"enabled":false}`, 200)
	var off ScheduleView
	decodeJSON(t, b, &off)
	if off.Enabled {
		t.Fatalf("rescan still on: %+v", off)
	}
	var listed schedulesAnswer
	decodeJSON(t, do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &listed)
	if got := scheduleNamed(t, listed, "rescan_library"); got.State != "off" || got.Enabled {
		t.Fatalf("listed rescan %+v", got)
	}
	b = do(t, "PUT", f.srv.URL+"/api/v1/schedules/rescan_library", `{"enabled":true}`, 200)
	var on ScheduleView
	decodeJSON(t, b, &on)
	if !on.Enabled || on.NextRun == nil {
		t.Fatalf("rescan back on: %+v", on)
	}
	// Movie details refresh can still be turned off; other kinds cannot.
	do(t, "PUT", f.srv.URL+"/api/v1/schedules/refresh_titles", `{"enabled":false}`, 200)
	for _, kind := range []string{"refresh_shows", "check_library_folders", "feed_check"} {
		do(t, "PUT", f.srv.URL+"/api/v1/schedules/"+kind, `{"enabled":false}`, 409)
	}
}

// The rescan is on and daily by default.
func TestRescanScheduleDefaultsOnAndDaily(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	var got schedulesAnswer
	if err := json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	rs := scheduleNamed(t, got, "rescan_library")
	if !rs.Enabled || rs.EverySeconds != 86400 {
		t.Fatalf("rescan default %+v", rs)
	}
}

// The folder check's About line says the write and hardlink test runs at
// start-up, on Run now and when folders change, not on every timed run.
func TestFolderCheckAboutSaysWhenTheWriteTestRuns(t *testing.T) {
	t.Parallel()
	about := strings.ToLower(ScheduleInfos["check_library_folders"].About)
	for _, want := range []string{"start-up", "run now", "hardlink"} {
		if !strings.Contains(about, want) {
			t.Errorf("About %q does not mention %q", ScheduleInfos["check_library_folders"].About, want)
		}
	}
}
