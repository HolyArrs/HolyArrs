package accounts

import (
	"sync"
	"time"
)

// pluginInflightMax is how many calls one plugin may have open at once.
const pluginInflightMax = 8

// activityKeep is how many recent calls each plugin's activity log keeps.
const activityKeep = 100

// PluginCall is one line of a plugin's activity log.
type PluginCall struct {
	At      time.Time `json:"at"`
	Method  string    `json:"method"`
	Path    string    `json:"path"`
	Outcome string    `json:"outcome"` // allowed, refused, limited
}

var (
	actMu    sync.Mutex
	activity = map[string][]PluginCall{}
	inflight = map[string]int{}
)

func recordPluginCall(id, method, path, outcome string, now time.Time) {
	actMu.Lock()
	defer actMu.Unlock()
	l := append(activity[id], PluginCall{At: now.UTC(), Method: method, Path: path, Outcome: outcome})
	if len(l) > activityKeep {
		l = l[len(l)-activityKeep:]
	}
	activity[id] = l
}

// PluginActivity is id's recent calls, newest first, and how many of them
// were refused or over the limit.
func PluginActivity(id string) (calls []PluginCall, refused, limited int) {
	actMu.Lock()
	defer actMu.Unlock()
	l := activity[id]
	calls = make([]PluginCall, 0, len(l))
	for i := len(l) - 1; i >= 0; i-- {
		calls = append(calls, l[i])
		switch l[i].Outcome {
		case "refused":
			refused++
		case "limited":
			limited++
		}
	}
	return calls, refused, limited
}

// enterPlugin takes one of id's open-call places; false when all are taken.
func enterPlugin(id string) bool {
	actMu.Lock()
	defer actMu.Unlock()
	if inflight[id] >= pluginInflightMax {
		return false
	}
	inflight[id]++
	return true
}

func leavePlugin(id string) {
	actMu.Lock()
	defer actMu.Unlock()
	if inflight[id]--; inflight[id] <= 0 {
		delete(inflight, id)
	}
}
