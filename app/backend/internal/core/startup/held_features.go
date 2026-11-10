package startup

import (
	"sync"
)

// The held-back features, by switch name.
const (
	AIApps            = "ai_apps"             // AI connector, AI keys, AI app guides
	RowTitleStats     = "row_title_stats"     // Explore row titles statistics (owner table)
	TraktLists        = "trakt_lists"         // Trakt key, Trakt lists, VIP token
	AlertChannelsMore = "alert_channels_more" // alert channels not yet proven end to end
	SubtitleSites     = "subtitle_sites"      // Subtitle sites and subtitle searching
	EmbySignIn        = "emby_signin"         // Sign in with Emby
	GoBack            = "go_back"             // Go back to the previous version steps
)

// ProvenAlertKinds are the alert channel kinds that stay on when
// AlertChannelsMore is off.
var ProvenAlertKinds = map[string]bool{"discord": true, "email": true}

// Register adds the held-back feature names to the registry. The program
// wiring calls it once; calling it again does nothing.
func RegisterHeldFeatures() {
	registerOnce.Do(func() {
		for _, n := range []string{AIApps, RowTitleStats, TraktLists, AlertChannelsMore, SubtitleSites, EmbySignIn, GoBack} {
			RegisterFeature(n)
		}
	})
}

var registerOnce sync.Once
