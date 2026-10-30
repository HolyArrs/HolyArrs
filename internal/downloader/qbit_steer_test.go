package downloader

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const steerA, steerB = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func steerFake(t *testing.T, api string) (*manageFake, *Qbittorrent) {
	t.Helper()
	f := &manageFake{answers: map[string]string{"app/webapiVersion": api}, codes: map[string]int{}}
	return f, NewQbittorrent(f.start(t), nil)
}

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
