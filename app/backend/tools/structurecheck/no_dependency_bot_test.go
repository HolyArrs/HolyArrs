package structurecheck

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNoDependencyBotConfig fails if a GitHub dependency bot config is back.
func TestNoDependencyBotConfig(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..")
	if _, err := os.Stat(filepath.Join(root, ".github")); err != nil {
		t.Fatalf("repo root not found at %s: %v", root, err)
	}
	for _, name := range []string{"dependabot.yml", "dependabot.yaml"} {
		p := filepath.Join(root, ".github", name)
		if _, err := os.Stat(p); err == nil {
			t.Errorf(".github/%s exists; the GitHub dependency bot must stay off", name)
		}
	}
}
