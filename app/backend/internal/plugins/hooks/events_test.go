package hooks

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestEventScopeTable(t *testing.T) {
	for _, c := range []struct {
		topic, data string
		need        string // "" = never delivered
	}{
		{"title", `{"id":1}`, "library.read"},
		{"slot", `{"id":1}`, "library.read"},
		{"download", `{"id":1}`, "downloads.read"},
		{"job", `{"id":1}`, "system.jobs.read"},
		{"health", `{}`, "system.health.read"},
		{"ask", `{"id":1}`, "requests.read"},
		{"ask_removed", `{"id":1}`, "requests.read"},
		{"event", `{"kind":"ask_made","summary":"x"}`, "requests.read"},
		{"event", `{"kind":"ask_madeup_secret","summary":"x"}`, ""},
		{"event", `{"kind":"import_done","summary":"x"}`, "downloads.read"},
		{"event", `{"kind":"import_","summary":"x"}`, ""},
		{"event", `{"kind":"title_added_by_ip","summary":"x"}`, ""},
		{"event", `{"kind":"sign_in_failed","summary":"x"}`, ""},
		{"event", `{"kind":"","summary":"x"}`, ""},
		{"event", `{"kind":"title_added","summary":"x"}`, "library.read"},
		{"event", `{"kind":"signin_failed","summary":"x"}`, ""},
		{"event", `{"kind":"key_made","summary":"x"}`, ""},
		{"x", `{}`, ""},
		{"settings", `{}`, ""},
	} {
		if got, _ := eventScope(c.topic, json.RawMessage(c.data)); got != c.need {
			t.Errorf("%s %s: needs %q, want %q", c.topic, c.data, got, c.need)
		}
	}
}

func TestEventsDeliveredOnlyForReadableAreasWithoutPrivateFields(t *testing.T) {
	b := newTestBroker(map[string][]string{
		"lib": {"alerts.events.subscribe", "library.read"},
		"req": {"alerts.events.subscribe", "requests.read", "library.read"},
		"no":  {"library.read"},
	})
	for _, p := range []string{"lib", "req", "no"} {
		b.mu.Lock()
		b.laneLocked(p, Events).seen = b.now()
		b.mu.Unlock()
	}
	r := &Registry{Broker: b}
	r.Publish("event", json.RawMessage(`{"kind":"ask_made","summary":"Dune","actor":{"name":"ann","ip":"10.0.0.9"},"data":{"ip":"10.0.0.9"}}`))
	r.Publish("event", json.RawMessage(`{"kind":"title_added","title_id":4,"summary":"Dune"}`))
	got := map[string][]string{}
	for _, p := range []string{"lib", "req"} {
		for {
			j, ok, _ := b.Next(context.Background(), p, Events, 10*time.Millisecond)
			if !ok {
				break
			}
			if strings.Contains(string(j.Payload), "10.0.0.9") || strings.Contains(string(j.Payload), "ann") {
				t.Errorf("%s got private fields: %s", p, j.Payload)
			}
			var e PluginEvent
			_ = json.Unmarshal(j.Payload, &e)
			got[p] = append(got[p], e.Kind)
		}
	}
	if !slices.Equal(got["lib"], []string{"title_added"}) || !slices.Equal(got["req"], []string{"ask_made", "title_added"}) {
		t.Errorf("delivered %v", got)
	}
	if _, _, err := b.Next(context.Background(), "no", Events, time.Millisecond); err == nil {
		t.Error("plugin without the subscribe permission polled events")
	}
}
