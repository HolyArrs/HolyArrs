package startup

import (
	"sync"
)

// The held-back features, by switch name.
const (
	AIApps        = "ai_apps"         // AI connector (MCP) and AI keys
	RowTitleStats = "row_title_stats" // Explore row titles statistics (owner table)
	GoBack        = "go_back"         // Go back to the previous version steps
)

// Register adds the held-back feature names to the registry. The program
// wiring calls it once; calling it again does nothing.
func RegisterHeldFeatures() {
	registerOnce.Do(func() {
		for _, n := range []string{AIApps, RowTitleStats, GoBack} {
			RegisterFeature(n)
		}
	})
}

var registerOnce sync.Once
