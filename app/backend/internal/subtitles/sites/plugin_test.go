package sites

import (
	"context"
	"errors"
	"testing"
)

type stubSite struct{ id string }

func (stubSite) Search(context.Context, Query) ([]Candidate, error)  { return nil, nil }
func (stubSite) Download(context.Context, Candidate) ([]byte, error) { return nil, nil }

// TestPluginSite: kind plugin takes plugin://<id> only, and needs the app to
// have set PluginSite.
func TestPluginSite(t *testing.T) {
	for raw, want := range map[string]string{
		"plugin://whisper-subtitles": "whisper-subtitles",
		"plugin://Whisper":           "",
		"plugin://ws/path":           "",
		"plugin://ws?x=1":            "",
		"plugin://me@ws":             "",
		"plugin:ws":                  "",
		"https://ws":                 "",
		"plugin://":                  "",
	} {
		if got := PluginOf(raw); got != want {
			t.Errorf("PluginOf(%q) = %q, want %q", raw, got, want)
		}
	}
	old := PluginSite
	t.Cleanup(func() { PluginSite = old })
	PluginSite = nil
	// Not set up yet, as before the app wires it: a Job waits for the plugin.
	if _, err := New("plugin", Config{URL: "plugin://ws"}); !errors.Is(err, ErrPluginGone) {
		t.Fatalf("no PluginSite: %v", err)
	}
	PluginSite = func(id string) Site { return stubSite{id: id} }
	s, err := New("plugin", Config{URL: "plugin://ws"})
	if err != nil || s.(stubSite).id != "ws" {
		t.Fatalf("got %v, %v", s, err)
	}
	if _, err := New("plugin", Config{URL: "https://ws.example"}); err == nil {
		t.Fatal("an http address for kind plugin: want an error")
	}
	if _, err := New("subdl", Config{URL: "plugin://ws"}); err == nil {
		t.Fatal("a plugin address for another kind: want an error")
	}
	if k, ok := KindOf("plugin"); !ok || !k.Movies || !k.Shows || k.NeedsAccount || k.DefaultURL != "" {
		t.Fatalf("kind %+v", k)
	}
}
