package hooks

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Metadata is the hook a plugin answers title ratings on (sources.metadata).
const Metadata = "metadata"

func init() { Scopes[Metadata] = "sources.metadata" }

// notifyPerHour is how many messages one plugin may send through the
// owner's alerts in an hour.
const notifyPerHour = 20

type notifyLimit struct {
	mu   sync.Mutex
	sent map[string][]time.Time
}

func (n *notifyLimit) allow(plugin string, now time.Time) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.sent == nil {
		n.sent = map[string][]time.Time{}
	}
	keep := n.sent[plugin][:0]
	for _, t := range n.sent[plugin] {
		if now.Sub(t) < time.Hour {
			keep = append(keep, t)
		}
	}
	if len(keep) >= notifyPerHour {
		n.sent[plugin] = keep
		return false
	}
	n.sent[plugin] = append(keep, now)
	return true
}

// artwork sets a title's poster or backdrop from a picture the plugin
// sends as bytes (library.artwork.set); a link is never fetched.
func (h *Handlers) artwork(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.pluginWith(w, r, "library.artwork.set"); !ok {
		return
	}
	var in struct {
		TitleID int64  `json:"title_id"`
		Kind    string `json:"kind"`
		Image   []byte `json:"image"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 12<<20)).Decode(&in); err != nil || in.TitleID <= 0 {
		webserver.Fail(w, http.StatusBadRequest, `send {"title_id": 12, "kind": "poster", "image": "<base64 JPEG or PNG>"}`)
		return
	}
	if h.SetArtwork == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "not available")
		return
	}
	if err := h.SetArtwork(r.Context(), in.TitleID, in.Kind, in.Image); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// notify sends one message through the owner's alerts that take plugin
// messages (alerts.webhooks.send), named as coming from the plugin.
func (h *Handlers) notify(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pluginWith(w, r, "alerts.webhooks.send")
	if !ok {
		return
	}
	var in struct {
		Subject string `json:"subject"`
		Text    string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in); err != nil || !shortText(in.Subject, 100, true) || !shortText(in.Text, 1000, false) {
		webserver.Fail(w, http.StatusBadRequest, `send {"subject": "...", "text": "..."}: up to 100 and 1000 characters, no markup`)
		return
	}
	if h.Notify == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "not available")
		return
	}
	if !h.notified.allow(id, time.Now()) {
		webserver.Fail(w, http.StatusTooManyRequests, "this plugin sent too many messages this hour")
		return
	}
	if err := h.Notify(r.Context(), id, in.Subject, in.Text); err != nil {
		webserver.Fail(w, http.StatusBadGateway, "the message could not be sent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func shortText(s string, n int, need bool) bool {
	if need && strings.TrimSpace(s) == "" {
		return false
	}
	return utf8.RuneCountInString(s) <= n && !strings.ContainsAny(s, "<>\x00")
}

// PluginRatings asks each metadata plugin for a title's ratings, briefly; a
// slow or failing plugin is left out, and each score is checked.
func (r *Registry) PluginRatings(ctx context.Context, kind, id string) []metadata.ServerScore {
	var out []metadata.ServerScore
	for _, p := range r.Broker.Providers(Metadata) {
		raw, err := r.Broker.Call(ctx, p, Metadata, "ratings", map[string]string{"kind": kind, "id": id}, 2*time.Second)
		if err != nil {
			continue
		}
		var got struct {
			Scores []metadata.ServerScore `json:"scores"`
		}
		if json.Unmarshal(raw, &got) != nil {
			continue
		}
		for i, s := range got.Scores {
			if i >= 5 {
				break
			}
			if ok := checkScore(s); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func checkScore(s metadata.ServerScore) bool {
	return shortText(s.Source, 30, true) && s.Max > 0 && s.Max <= 100 && s.Value >= 0 && s.Value <= s.Max &&
		!math.IsNaN(s.Value) && s.Votes >= 0
}

// WithPluginRatings adds the plugins' ratings to a ratings reader. A source
// the server already rates is never replaced by a plugin.
func (r *Registry) WithPluginRatings(next func(ctx context.Context, kind, id string) (metadata.ServerRatings, error)) func(ctx context.Context, kind, id string) (metadata.ServerRatings, error) {
	return func(ctx context.Context, kind, id string) (metadata.ServerRatings, error) {
		var rs metadata.ServerRatings
		var err error
		if next != nil {
			rs, err = next(ctx, kind, id)
		}
		extra := r.PluginRatings(ctx, kind, id)
		if err != nil && len(extra) == 0 {
			return rs, err
		}
		have := map[string]bool{}
		for _, s := range rs.Scores {
			have[strings.ToLower(s.Source)] = true
		}
		for _, s := range extra {
			if !have[strings.ToLower(s.Source)] {
				have[strings.ToLower(s.Source)] = true
				rs.Scores = append(rs.Scores, s)
			}
		}
		return rs, nil
	}
}
