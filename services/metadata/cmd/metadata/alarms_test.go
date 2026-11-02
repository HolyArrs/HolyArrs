package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
)

// ALARM lines are counted (same text but numbers), kept across a restart,
// and drop out of the label after a day; other lines are not noted.
func TestAlarmLogKeptAndCounted(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	a := openAlarmLog(dir)
	a.now = func() time.Time { return now }
	w := a.tap(io.Discard)
	io.WriteString(w, "2026/10/31 12:00:00 fetchq images ALARM: 3 connection failures in a row\n")
	io.WriteString(w, "2026/10/31 12:00:05 music sources: count failed\n")
	io.WriteString(w, "2026/10/31 12:01:00 fetchq images ALARM: 5 connection failures in a row\n")
	got := a.recent()
	if len(got) != 1 || got[0].Count != 2 || !strings.HasPrefix(got[0].Text, "fetchq images ALARM: 5") {
		t.Fatalf("recent: %+v", got)
	}
	b := openAlarmLog(dir) // a restart
	b.now = a.now
	if len(b.recent()) != 1 {
		t.Fatal("not kept across a restart")
	}
	b.now = func() time.Time { return now.Add(25 * time.Hour) }
	if len(b.recent()) != 0 {
		t.Fatal("an alarm older than a day still shown")
	}
}

// The checks: an old slim copy, no copy, and a missing or old backup.
func TestAlarmChecks(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	kinds := func(items []alarmItem) string {
		var k []string
		for _, i := range items {
			k = append(k, i.Kind)
		}
		return strings.Join(k, ",")
	}
	c := alarmChecks{dataDir: dir, slimDate: func() string { return "20261028-002228" }, now: func() time.Time { return now }}
	if got := kinds(c.items()); got != "backup" {
		t.Fatalf("no backup: %q", got)
	}
	os.MkdirAll(filepath.Join(dir, "backups"), 0o755)
	f := filepath.Join(dir, "backups", "permanent-20261031T030000.tar.gz")
	os.WriteFile(f, []byte("x"), 0o600)
	os.Chtimes(f, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	if got := kinds(c.items()); got != "" {
		t.Fatalf("all fine: %q", got)
	}
	os.Chtimes(f, now.Add(-40*time.Hour), now.Add(-40*time.Hour))
	c.slimDate = func() string { return "20261015-002228" }
	if got := kinds(c.items()); got != "slim,backup" {
		t.Fatalf("old copy and backup: %q", got)
	}
	c.slimDate = func() string { return "" }
	if items := c.items(); items[0].Kind != "slim" || !strings.Contains(items[0].Text, "no slim") {
		t.Fatalf("no copy: %+v", items)
	}
}

// The status block: ok with no items, else the items, checks first.
func TestAlarmsStatusBlock(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	os.MkdirAll(filepath.Join(dir, "backups"), 0o755)
	os.WriteFile(filepath.Join(dir, "backups", "permanent-x.tar.gz"), []byte("x"), 0o600)
	logs := openAlarmLog("")
	o := &api.Options{}
	openAlarms(logs, alarmChecks{dataDir: dir, now: func() time.Time { return now }}, o)
	block := o.StatusExtra(context.Background(), time.Hour)["alarms"].(map[string]any)
	if block["ok"] != true || len(block["items"].([]alarmItem)) != 0 {
		t.Fatalf("clean: %+v", block)
	}
	logs.note("upstream ALARM: tmdb failed to connect 3 times")
	block = o.StatusExtra(context.Background(), time.Hour)["alarms"].(map[string]any)
	items := block["items"].([]alarmItem)
	if block["ok"] != false || len(items) != 1 || items[0].Kind != "log" || items[0].Count != 1 {
		t.Fatalf("one alarm: %+v", block)
	}
}

// The music check: quiet under 90% of the cap, near it, and over it.
func TestAlarmMusicCap(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "backups"), 0o755)
	os.WriteFile(filepath.Join(dir, "backups", "permanent-x.tar.gz"), []byte("x"), 0o600)
	var n int64
	c := alarmChecks{dataDir: dir, music: func() int64 { return n }, now: time.Now}
	for _, tc := range []struct {
		used int64
		want string
	}{{0, ""}, {80 << 30, ""}, {82 << 30, "raise the cap"}, {91 << 30, "stopped"}} {
		n = tc.used
		var got string
		for _, i := range c.items() {
			if i.Kind == "music" {
				got = i.Text
			}
		}
		if (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Fatalf("%d bytes: %q", tc.used, got)
		}
	}
}
