package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
)

// The "alarms" block of the status answer, shown as a label in HolyDash:
// the log's ALARM lines of the last day (kept in the data folder, so a
// deploy does not lose them) and the checks made when asked: the slim
// MusicBrainz copy's age, music data near its cap, free disk, and the
// newest backup's age.

const (
	alarmWindow    = 24 * time.Hour
	alarmKeep      = 50
	slimMaxAge     = 10 * 24 * time.Hour
	diskMinFree    = 0.15
	musicNearCap   = 0.9
	backupMaxAge   = 36 * time.Hour
	alarmFileName  = "alarms.json"
	alarmTextLimit = 240
)

// alarm is one ALARM line, repeated lines counted (same text without its
// numbers).
type alarm struct {
	Text  string    `json:"text"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
	Count int       `json:"count"`
}

// alarmLog keeps the ALARM lines written to the log.
type alarmLog struct {
	path string // "" keeps none
	now  func() time.Time

	mu    sync.Mutex
	items []alarm
}

var (
	logStamp   = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)
	alarmDigit = regexp.MustCompile(`\d+`)
)

func openAlarmLog(dir string) *alarmLog {
	a := &alarmLog{now: time.Now}
	if dir != "" {
		a.path = filepath.Join(dir, alarmFileName)
		if b, err := os.ReadFile(a.path); err == nil {
			_ = json.Unmarshal(b, &a.items)
		}
	}
	return a
}

// tap passes the log through and notes each ALARM line.
func (a *alarmLog) tap(w io.Writer) io.Writer { return alarmTap{w, a} }

type alarmTap struct {
	w io.Writer
	a *alarmLog
}

func (t alarmTap) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if bytes.Contains(p, []byte("ALARM")) {
		for _, line := range strings.Split(strings.TrimSpace(string(p)), "\n") {
			if strings.Contains(line, "ALARM") {
				t.a.note(logStamp.ReplaceAllString(line, ""))
			}
		}
	}
	return n, err
}

func (a *alarmLog) note(text string) {
	if len(text) > alarmTextLimit {
		text = text[:alarmTextLimit]
	}
	now := a.now().UTC()
	key := alarmDigit.ReplaceAllString(text, "#")
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.items {
		if alarmDigit.ReplaceAllString(a.items[i].Text, "#") == key {
			a.items[i].Text, a.items[i].Last = text, now
			a.items[i].Count++
			a.save()
			return
		}
	}
	a.items = append(a.items, alarm{Text: text, First: now, Last: now, Count: 1})
	if len(a.items) > alarmKeep {
		a.items = a.items[len(a.items)-alarmKeep:]
	}
	a.save()
}

// save writes the kept lines (under mu): aside, then renamed.
func (a *alarmLog) save() {
	if a.path == "" {
		return
	}
	b, err := json.Marshal(a.items)
	if err != nil {
		return
	}
	tmp := a.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, a.path) != nil {
		_ = os.Remove(tmp)
	}
}

// recent answers the lines seen in the last day, newest first.
func (a *alarmLog) recent() []alarm {
	since := a.now().Add(-alarmWindow)
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []alarm
	for i := len(a.items) - 1; i >= 0; i-- {
		if a.items[i].Last.After(since) {
			out = append(out, a.items[i])
		}
	}
	return out
}

// alarmChecks are the checks made when the status is asked.
type alarmChecks struct {
	dataDir  string
	slimDate func() string // "20261028-002228", "" when none
	music    func() int64  // music data bytes at the last cap check (0: unknown)
	now      func() time.Time
}

type alarmItem struct {
	Kind  string     `json:"kind"`
	Text  string     `json:"text"`
	First *time.Time `json:"first,omitempty"`
	Last  *time.Time `json:"last,omitempty"`
	Count int        `json:"count,omitempty"`
}

func (c alarmChecks) items() []alarmItem {
	var out []alarmItem
	now := c.now()
	if c.slimDate != nil {
		if d := c.slimDate(); d == "" {
			out = append(out, alarmItem{Kind: "slim", Text: "no slim MusicBrainz copy loaded"})
		} else if t, err := time.Parse("20060102-150405", d); err == nil && now.Sub(t) > slimMaxAge {
			out = append(out, alarmItem{Kind: "slim", Text: fmt.Sprintf("slim MusicBrainz copy is %d days old (%s); run mbimport", int(now.Sub(t).Hours()/24), d)})
		}
	}
	if c.music != nil {
		if n := c.music(); n > mbdump.CapBytes {
			out = append(out, alarmItem{Kind: "music", Text: fmt.Sprintf("music data at %.1f of %d GB; new music pictures stopped", float64(n)/(1<<30), mbdump.CapBytes>>30)})
		} else if float64(n) > musicNearCap*float64(mbdump.CapBytes) {
			out = append(out, alarmItem{Kind: "music", Text: fmt.Sprintf("music data at %.1f of %d GB; raise the cap or new music pictures stop", float64(n)/(1<<30), mbdump.CapBytes>>30)})
		}
	}
	if used, total, err := refresh.DiskUsage(c.dataDir); err == nil && total > 0 {
		if free := float64(total-used) / float64(total); free < diskMinFree {
			out = append(out, alarmItem{Kind: "disk", Text: fmt.Sprintf("disk %.0f%% free (%d GB); pictures are being evicted", free*100, (total-used)>>30)})
		}
	}
	files, _ := filepath.Glob(filepath.Join(c.dataDir, "backups", "permanent-*.tar.gz"))
	var newest time.Time
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	if newest.IsZero() {
		out = append(out, alarmItem{Kind: "backup", Text: "no permanent backup on the server"})
	} else if age := now.Sub(newest); age > backupMaxAge {
		out = append(out, alarmItem{Kind: "backup", Text: fmt.Sprintf("newest backup is %d hours old; check the homelab pull", int(age.Hours()))})
	}
	return out
}

// openAlarms adds the "alarms" block to the status answer: ok, and each
// open item (checks first, then the day's log alarms).
func openAlarms(logs *alarmLog, checks alarmChecks, o *api.Options) {
	prev := o.StatusExtra
	o.StatusExtra = func(ctx context.Context, window time.Duration) map[string]any {
		m := map[string]any{}
		if prev != nil {
			m = prev(ctx, window)
		}
		items := checks.items()
		for _, l := range logs.recent() {
			first, last := l.First, l.Last
			items = append(items, alarmItem{Kind: "log", Text: l.Text, First: &first, Last: &last, Count: l.Count})
		}
		if items == nil {
			items = []alarmItem{}
		}
		m["alarms"] = map[string]any{"ok": len(items) == 0, "items": items}
		return m
	}
}
