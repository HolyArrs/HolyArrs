package plugins

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// Stat is one plugin's public numbers from stats.json, next to the plugin
// list. Nothing about this install is sent: it is a plain GET of one file.
type Stat struct {
	Downloads *int64 `json:"downloads,omitempty"`
	Week      *int64 `json:"week,omitempty"`
	Stars     *int64 `json:"stars,omitempty"`
	Updated   string `json:"updated,omitempty"`
	// Listed is when the plugin was first listed (ISO time); the page shows
	// "New" from it. It lives here, not on the signed list, so older apps
	// that refuse unknown list fields are not affected.
	Listed string `json:"listed,omitempty"`
}

const statsEvery = time.Hour

// stats gives the last fetched numbers and, when they are older than an
// hour, fetches them again in the background so the list never waits.
// A missing or broken file just leaves the numbers out.
func (e *Engine) stats() map[string]Stat {
	e.statMu.Lock()
	defer e.statMu.Unlock()
	if !e.statBusy && time.Since(e.statAt) > statsEvery {
		e.statBusy = true
		go e.fetchStats()
	}
	return e.statData
}

func (e *Engine) fetchStats() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	raw, err := e.fetch(ctx, "stats.json")
	var m map[string]Stat
	if err == nil {
		err = json.Unmarshal(raw, &m)
	}
	e.statMu.Lock()
	defer e.statMu.Unlock()
	e.statBusy, e.statAt = false, time.Now()
	if err != nil {
		log.Printf("plugins: download numbers skipped: %v", err)
		e.statData = nil
		return
	}
	e.statData = m
}
