package hooks

import (
	"encoding/json"
	"time"
)

// topicScopes are the read scopes a live topic needs. A topic not listed
// is never handed to a plugin.
var topicScopes = map[string]string{
	"title":          "library.read",
	"slot":           "library.read",
	"library_folder": "library.read",
	"download":       "downloads.read",
	"downloader":     "downloads.clients.read",
	"source":         "downloads.sources.read",
	"job":            "system.jobs.read",
	"health":         "system.health.read",
	"outside_list":   "discover.lists.read",
	"ask":            "requests.read",
	"ask_removed":    "requests.read",
}

// eventKinds are the history event kinds handed out, each exactly, with
// the scope it needs. Any other kind (sign-in, keys, accounts, settings,
// or one added later) is never handed to a plugin.
var eventKinds = map[string]string{
	"ask_made": "requests.read", "ask_available": "requests.read",
	"import_done": "downloads.read", "import_failed": "downloads.read", "import_partly_done": "downloads.read", "import_rejected": "downloads.read",
	"grabbed": "downloads.read", "release_chosen": "downloads.read",
	"download_sent": "downloads.read", "download_failed": "downloads.read", "download_vanished": "downloads.read",
	"download_changed": "downloads.read", "download_downloading": "downloads.read",
	"title_added": "library.read", "renamed": "library.read", "file_replaced": "library.read",
	"disk_space": "system.health.read",
}

// PluginEvent is what a plugin receives: what happened and to which item,
// never who did it, from where, or the event's inner data.
type PluginEvent struct {
	Topic      string    `json:"topic"`
	Kind       string    `json:"kind,omitempty"`
	At         time.Time `json:"at"`
	ID         int64     `json:"id,omitempty"`
	TitleID    int64     `json:"title_id,omitempty"`
	SlotID     int64     `json:"slot_id,omitempty"`
	DownloadID int64     `json:"download_id,omitempty"`
	Summary    string    `json:"summary,omitempty"`
}

// eventScope is the read scope a plugin needs for one live message, and
// what it may see of it; "" means it is never delivered.
func eventScope(topic string, data json.RawMessage) (string, PluginEvent) {
	var in struct {
		ID         int64  `json:"id"`
		Kind       string `json:"kind"`
		TitleID    int64  `json:"title_id"`
		SlotID     int64  `json:"slot_id"`
		DownloadID int64  `json:"download_id"`
		Summary    string `json:"summary"`
	}
	_ = json.Unmarshal(data, &in)
	ev := PluginEvent{Topic: topic, At: time.Now().UTC(), ID: in.ID, TitleID: in.TitleID, SlotID: in.SlotID, DownloadID: in.DownloadID}
	if topic != "event" {
		return topicScopes[topic], ev
	}
	ev.Kind, ev.Summary, ev.ID = clip(in.Kind, 60), clip(in.Summary, 300), 0
	return eventKinds[in.Kind], ev
}

// Publish hands one live message to every plugin subscribed to events
// that can read its area, judged by the scopes it holds right now.
func (r *Registry) Publish(topic string, data json.RawMessage) {
	need, ev := eventScope(topic, data)
	if need == "" {
		return
	}
	b := r.Broker
	for _, p := range b.Providers(Events) {
		if b.Granted != nil && b.Covers(b.Granted(p), need) {
			b.Send(p, Events, "event", ev)
		}
	}
}
