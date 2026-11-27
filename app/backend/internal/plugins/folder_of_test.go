package plugins_test

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFolderOf: a file inside a folder the plugin was given is named by that
// folder and the path inside it; a file elsewhere, or a link out of the
// folder, is not.
func TestFolderOf(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("folders are opened only on Linux, where plugins run")
	}
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	ls.publishWith("1.0.1", []byte("v101"), 2, func(man map[string]any) {
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "actions": []string{"sources.subtitles"},
			"folders": []any{map[string]any{"name": "movies", "mode": "read", "required": true}}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	root := t.TempDir()
	movies, other := filepath.Join(root, "movies"), filepath.Join(root, "other")
	for _, d := range []string{filepath.Join(movies, "Arrival (2016)"), other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if name, _, ok := e.FolderOf("tailscale", filepath.Join(movies, "Arrival (2016)", "Arrival.mkv")); ok {
		t.Fatalf("before approval: %q", name)
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{"movies":`+jsonStr(movies)+`}}`, http.StatusOK)
	for _, c := range []struct {
		file, name, rel string
		ok              bool
	}{
		{filepath.Join(movies, "Arrival (2016)", "Arrival.mkv"), "movies", "Arrival (2016)/Arrival.mkv", true},
		{filepath.Join(other, "Arrival.mkv"), "", "", false},
		{movies + "-not/Arrival.mkv", "", "", false},
		{filepath.Join(movies, "..", "other", "Arrival.mkv"), "", "", false},
	} {
		name, rel, ok := e.FolderOf("tailscale", c.file)
		if name != c.name || rel != c.rel || ok != c.ok {
			t.Errorf("FolderOf(%q) = %q %q %v, want %q %q %v", c.file, name, rel, ok, c.name, c.rel, c.ok)
		}
	}
	if _, _, ok := e.FolderOf("other-plugin", filepath.Join(movies, "Arrival (2016)", "Arrival.mkv")); ok {
		t.Fatal("another plugin's folder")
	}
}
