// Package control is the kill switch (music speed plan, section 7): one
// small JSON file on the server, read every 30 s, served to installs as a
// cacheable static answer and obeyed by the server's own workers.
package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"sync/atomic"
	"time"
)

// Values are the switch settings.
type Values struct {
	// Prefetch is "on", "slow" or "off" for install prefetch and hover fetch.
	Prefetch         string          `json:"prefetch"`
	PrebuildMinHours float64         `json:"prebuild_min_hours"`
	MaxRPS           float64         `json:"max_rps"`
	Providers        map[string]bool `json:"providers"`
	Queue            map[string]bool `json:"queue"`
	// P0MaxRPS caps how many user-driven misses reach MusicBrainz.
	P0MaxRPS float64 `json:"p0_max_rps"`
}

// MBScale multiplies the MusicBrainz defaults (MaxRPS, P0MaxRPS) when
// calls are spread over several outgoing addresses.
var MBScale = 1.0

// Defaults are used when the file is missing or unreadable.
func Defaults() Values {
	return Values{Prefetch: "on", PrebuildMinHours: 3, MaxRPS: MBScale, P0MaxRPS: 0.54 * MBScale,
		Providers: map[string]bool{"caa": true, "fanart": true, "deezer": true, "wikidata": true, "youtube": true},
		Queue:     map[string]bool{"p2": true, "p3": true, "p4": true}}
}

type state struct {
	v    Values
	body []byte
	etag string
}

// Switch holds the current values.
type Switch struct {
	Path string
	cur  atomic.Pointer[state]
}

// New reads path once (missing file: defaults).
func New(path string) *Switch {
	s := &Switch{Path: path}
	s.Reload()
	return s
}

// Reload reads the file again; a bad file keeps the last good values.
func (s *Switch) Reload() {
	v := Defaults()
	if s.Path != "" {
		if b, err := os.ReadFile(s.Path); err == nil {
			if err := json.Unmarshal(b, &v); err != nil {
				log.Printf("control: %s unreadable, keeping last values", s.Path)
				if s.cur.Load() != nil {
					return
				}
				v = Defaults()
			}
		}
	}
	if v.Prefetch != "on" && v.Prefetch != "slow" && v.Prefetch != "off" {
		v.Prefetch = "on"
	}
	if v.Providers == nil {
		v.Providers = Defaults().Providers
	}
	if v.Queue == nil {
		v.Queue = Defaults().Queue
	}
	body, _ := json.Marshal(v)
	h := sha256.Sum256(body)
	s.cur.Store(&state{v: v, body: body, etag: `"` + hex.EncodeToString(h[:8]) + `"`})
}

// Get answers the current values.
func (s *Switch) Get() Values {
	if s == nil {
		return Defaults()
	}
	return s.cur.Load().v
}

// Body answers the JSON and its ETag.
func (s *Switch) Body() ([]byte, string) {
	c := s.cur.Load()
	return c.body, c.etag
}

// Provider reports whether a provider is on (unknown names are on).
func (s *Switch) Provider(name string) bool {
	on, ok := s.Get().Providers[name]
	return !ok || on
}

// Lane reports whether a queue lane (p2, p3, p4) runs.
func (s *Switch) Lane(name string) bool {
	on, ok := s.Get().Queue[name]
	return !ok || on
}

// Run re-reads the file every 30 s.
func (s *Switch) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Reload()
		}
	}
}
