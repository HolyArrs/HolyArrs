package downloads

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const steerA, steerB = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

const trackerRows = `[{"url":"** [DHT] **","tier":-1,"status":2},{"url":"** [PeX] **","tier":-1,"status":0},{"url":"https://t.example/abc123/announce","tier":0,"status":2},{"url":"udp://open.example:80","tier":1,"status":4}]`

func TestShareLimits52SendsAction(t *testing.T) {
	f, q := steerFake(t, "2.15.1")
	if err := q.SetShareLimits(context.Background(), []string{steerA, strings.ToUpper(steerB)}, ShareLimits{Ratio: 1.5, SeedingMinutes: 600, InactiveMinutes: -1, Action: "remove"}); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/setShareLimits"); got != "torrents/setShareLimits?hashes="+steerA+"%7C"+steerB+"&inactiveSeedingTimeLimit=-1&ratioLimit=1.5&seedingTimeLimit=600&shareLimitAction=Remove" {
		t.Fatal(got)
	}
	_ = q.SetShareLimits(context.Background(), []string{steerA}, ShareLimits{Ratio: -2, SeedingMinutes: -2, InactiveMinutes: -2})
	if got := f.last("torrents/setShareLimits"); !strings.Contains(got, "shareLimitAction=Default") || !strings.Contains(got, "ratioLimit=-2") {
		t.Fatal(got)
	}
}

func TestShareLimits51NoAction(t *testing.T) {
	f, q := steerFake(t, "2.11.4")
	if err := q.SetShareLimits(context.Background(), []string{steerA}, ShareLimits{Ratio: 2, SeedingMinutes: -1, InactiveMinutes: 30, Action: "stop"}); !errors.Is(err, ErrShareAction) {
		t.Fatalf("5.1 action: %v", err)
	}
	if f.count("torrents/setShareLimits") != 0 {
		t.Fatal("sent anyway")
	}
	if err := q.SetShareLimits(context.Background(), []string{steerA}, ShareLimits{Ratio: 2, SeedingMinutes: -1, InactiveMinutes: 30}); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/setShareLimits"); strings.Contains(got, "shareLimitAction") || !strings.Contains(got, "inactiveSeedingTimeLimit=30") {
		t.Fatal(got)
	}
	if err := q.SetShareLimits(context.Background(), []string{"all"}, ShareLimits{}); err == nil {
		t.Fatal("all passed")
	}
}

func TestShareLimitsRead(t *testing.T) {
	r, s := 1.0, int64(60)
	l := qbitTorrent{RatioLimit: &r, SeedLimit: &s, ShareAction: "RemoveWithContent"}.limits()
	if l == nil || l.Ratio != 1 || l.SeedingMinutes != 60 || l.InactiveMinutes != -2 || l.Action != "remove_files" {
		t.Fatalf("%+v", l)
	}
	if (qbitTorrent{}).limits() != nil {
		t.Fatal("no limits read as some")
	}
	idle := int64(30)
	if l := (qbitTorrent{IdleLimit: &idle}).limits(); l == nil || l.InactiveMinutes != 30 {
		t.Fatalf("idle limit alone lost: %+v", l)
	}
}

func TestQbitAPIVersionCachedAndFailureIsError(t *testing.T) {
	f, q := steerFake(t, "2.13.1")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		v, err := q.qbitAPI(ctx)
		if err != nil || v != [3]int{2, 13, 1} {
			t.Fatalf("%v %v", v, err)
		}
	}
	n := 0
	f.mu.Lock()
	for _, c := range f.calls {
		if strings.Contains(c, "app/webapiVersion") {
			n++
		}
	}
	f.mu.Unlock()
	if n != 1 {
		t.Fatalf("version read %d times, want 1", n)
	}
	q.verAt = time.Now().Add(-2 * qbitVersionTTL)
	f.mu.Lock()
	f.codes["app/webapiVersion"] = 500
	f.mu.Unlock()
	if _, err := q.qbitAPI(ctx); err == nil {
		t.Fatal("a failed version read must be an error")
	}
}

func TestQbitAPIAtLeastRechecksOnlyStaleOldVersion(t *testing.T) {
	f, q := steerFake(t, "2.11.4")
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if ok, err := q.qbitAPIAtLeast(ctx, 2, 13, 0); ok || err != nil {
			t.Fatalf("%v %v", ok, err)
		}
	}
	if n := f.count("app/webapiVersion"); n != 1 {
		t.Fatalf("fresh old version read %d times, want 1", n)
	}
	q.verAt = time.Now().Add(-3 * time.Minute)
	f.mu.Lock()
	f.answers["app/webapiVersion"] = "2.13.1"
	f.mu.Unlock()
	if ok, err := q.qbitAPIAtLeast(ctx, 2, 13, 0); !ok || err != nil {
		t.Fatalf("stale old version not read again: %v %v", ok, err)
	}
}

func TestSlowModeFlipsOnlyWhenDifferent(t *testing.T) {
	f, q := steerFake(t, "2.11.4")
	f.answers["transfer/speedLimitsMode"] = "0"
	if err := q.SetSlowMode(context.Background(), false); err != nil || f.count("transfer/toggleSpeedLimitsMode") != 0 {
		t.Fatalf("off to off: %v %d", err, f.count("transfer/toggleSpeedLimitsMode"))
	}
	if err := q.SetSlowMode(context.Background(), true); err != nil || f.count("transfer/toggleSpeedLimitsMode") != 1 {
		t.Fatalf("off to on: %v", err)
	}
	f.answers["transfer/speedLimitsMode"] = "1"
	_ = q.SetSlowMode(context.Background(), true)
	if f.count("transfer/toggleSpeedLimitsMode") != 1 {
		t.Fatal("on to on flipped")
	}
}

func TestStatusReadsConnection(t *testing.T) {
	f, q := steerFake(t, "2.15.1")
	f.answers["sync/maindata"] = `{"server_state":{"connection_status":"firewalled","use_alt_speed_limits":true}}`
	st, err := q.Status(context.Background())
	if err != nil || st.Connection != "firewalled" || st.SlowMode == nil || !*st.SlowMode {
		t.Fatalf("%+v %v", st, err)
	}
	f.answers["sync/maindata"] = `{"server_state":{"connection_status":"weird"}}`
	st, _ = q.Status(context.Background())
	if st.Connection != "" || st.SlowMode != nil {
		t.Fatalf("%+v", st)
	}
}

func TestJobColumns(t *testing.T) {
	j := qbitTorrent{Hash: steerA, DoneOn: 1700000000, QueuePos: 3, Tracker: "https://Tracker.Example:8443/abc123passkey/announce"}.job()
	if j.DoneAt != 1700000000 || j.QueuePos != 3 || j.Tracker != "tracker.example" {
		t.Fatalf("%+v", j)
	}
	if (qbitTorrent{DoneOn: -1, QueuePos: -1}).job().DoneAt != 0 {
		t.Fatal("unset done date")
	}
}

func TestTrackersFixedRowsAndRefs(t *testing.T) {
	f, q := steerFake(t, "2.15.1")
	f.answers["torrents/trackers"] = trackerRows
	tr, err := q.Trackers(context.Background(), steerA)
	if err != nil || len(tr) != 4 || tr[0].URL != "https://t.example" || tr[2].URL != "DHT" || tr[2].Status != "working" || tr[3].Status != "disabled" || tr[0].Ref == "" || tr[2].Ref != "" {
		t.Fatalf("%+v %v", tr, err)
	}
}

func TestTrackerEdits52(t *testing.T) {
	f, q := steerFake(t, "2.15.1")
	f.answers["torrents/trackers"] = trackerRows
	ctx := context.Background()
	if err := q.AddTrackers(ctx, steerA, []string{"udp://a.example:1", "https://b.example/x"}); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/addTrackers"); got != "torrents/addTrackers?hash="+steerA+"&urls=udp%3A%2F%2Fa.example%3A1%0Ahttps%3A%2F%2Fb.example%2Fx" {
		t.Fatal(got)
	}
	ref := trackerRef("https://t.example/abc123/announce")
	tier := 3
	if err := q.EditTracker(ctx, steerA, ref, "", &tier); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/editTracker"); !strings.Contains(got, "tier=3") || !strings.Contains(got, "url=https%3A%2F%2Ft.example%2Fabc123%2Fannounce") || strings.Contains(got, "origUrl") {
		t.Fatal(got)
	}
	if err := q.RemoveTrackers(ctx, steerA, []string{ref, trackerRef("udp://open.example:80")}); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/removeTrackers"); !strings.Contains(got, "urls=https%3A%2F%2Ft.example%2Fabc123%2Fannounce%7Cudp%3A%2F%2Fopen.example%3A80") {
		t.Fatal(got)
	}
	if err := q.RemoveTrackers(ctx, steerA, []string{"nope"}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestTrackerEdit51(t *testing.T) {
	f, q := steerFake(t, "2.11.4")
	f.answers["torrents/trackers"] = trackerRows
	ref := trackerRef("udp://open.example:80")
	tier := 2
	if err := q.EditTracker(context.Background(), steerA, ref, "udp://new.example:80", &tier); !errors.Is(err, ErrTrackerTier) {
		t.Fatal(err)
	}
	if err := q.EditTracker(context.Background(), steerA, ref, "udp://new.example:80", nil); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/editTracker"); !strings.Contains(got, "origUrl=udp%3A%2F%2Fopen.example%3A80") || !strings.Contains(got, "newUrl=udp%3A%2F%2Fnew.example%3A80") || strings.Contains(got, "tier") {
		t.Fatal(got)
	}
}

func TestTrackerAllowedTyped(t *testing.T) {
	if !TrackerAllowed("udp://tracker.example:80", false) || TrackerAllowed("udp://a.example|udp://b.example", true) || TrackerAllowed("http://127.0.0.1/a", false) || TrackerAllowed("ftp://x.example/", true) {
		t.Fatal("typed tracker rules")
	}
}

func TestTrackerProblem(t *testing.T) {
	for _, c := range []struct {
		t    qbitTorrent
		want string
	}{
		{qbitTorrent{TrackerErr: true, TrackerWarn: true}, "error"},
		{qbitTorrent{TrackerWarn: true}, "warning"},
		{qbitTorrent{OtherErr: true}, "announce"},
		{qbitTorrent{State: "stalledDL", Trackers: 2}, "announce"},
		{qbitTorrent{State: "stoppedDL", Trackers: 2}, ""},
		{qbitTorrent{State: "uploading", Trackers: 2, Tracker: "https://t.example/a"}, ""},
	} {
		if got := c.t.problem(); got != c.want {
			t.Errorf("%+v: %q", c.t, got)
		}
	}
}

func TestMoveAndCap(t *testing.T) {
	f, q := steerFake(t, "2.11.4")
	ctx := context.Background()
	if err := q.SetLocation(ctx, []string{steerA, steerB}, "/data/downloads/new"); err != nil {
		t.Fatal(err)
	}
	if got := f.last("torrents/setLocation"); got != "torrents/setLocation?hashes="+steerA+"%7C"+steerB+"&location=%2Fdata%2Fdownloads%2Fnew" {
		t.Fatal(got)
	}
	if err := q.SetTorrentLimits(ctx, []string{steerA}, 1024, 0); err != nil {
		t.Fatal(err)
	}
	if f.last("torrents/setDownloadLimit") != "torrents/setDownloadLimit?hashes="+steerA+"&limit=1024" || f.last("torrents/setUploadLimit") != "torrents/setUploadLimit?hashes="+steerA+"&limit=0" {
		t.Fatal(f.calls)
	}
	if q.SetLocation(ctx, []string{"all"}, "/x") == nil || q.SetLocation(ctx, []string{steerA}, "") == nil {
		t.Fatal("bad input passed")
	}
	if j := (qbitTorrent{DLLimit: 500, UPLimit: -1}).job(); j.LimitDown != 500 || j.LimitUp != 0 {
		t.Fatalf("%+v", j)
	}
}

func steerFake(t *testing.T, api string) (*manageFake, *Qbittorrent) {
	t.Helper()
	f := &manageFake{answers: map[string]string{"app/webapiVersion": api}, codes: map[string]int{}}
	return f, NewQbittorrent(f.start(t), nil)
}
