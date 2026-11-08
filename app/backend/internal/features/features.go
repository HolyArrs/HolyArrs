// Package features names the features held back from the first release and
// registers them with core/startup. A name moves to the module that owns the
// feature when that module moves.
package features

import "github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"

// The held-back features, by switch name.
const (
	AIApps            = "ai_apps"             // AI connector, AI keys, AI app guides
	PlexWatchlists    = "plex_watchlists"     // Plex watchlists become requests
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

func init() {
	for _, n := range []string{AIApps, PlexWatchlists, RowTitleStats, TraktLists, AlertChannelsMore, SubtitleSites, EmbySignIn, GoBack} {
		startup.RegisterFeature(n)
	}
}
