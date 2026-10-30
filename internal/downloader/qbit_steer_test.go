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
