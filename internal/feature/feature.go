// Package feature holds the switches for features held back from the first
// release (Luis, 2026-11-07: anything not fully proven waits behind a switch
// and comes back later, done). A held-back feature is off unless its name is
// listed in HOLYARRS_FLAGS (comma separated), the same list the music rows use.
// Off means: its routes are not registered (the server answers not found), its
// jobs do not run, and the web app hides every place that shows it. Data
// already saved for it is kept untouched.
package feature

import (
	"os"
	"strings"
)

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

// Held lists every held-back feature, in a fixed order.
var Held = []string{AIApps, PlexWatchlists, RowTitleStats, TraktLists, AlertChannelsMore, SubtitleSites, EmbySignIn, GoBack}

// ProvenAlertKinds are the alert channel kinds that stay on when
// AlertChannelsMore is off.
var ProvenAlertKinds = map[string]bool{"discord": true, "email": true}

// On reports whether a feature switch is listed in HOLYARRS_FLAGS.
func On(name string) bool {
	for _, f := range strings.Split(os.Getenv("HOLYARRS_FLAGS"), ",") {
		if strings.TrimSpace(f) == name {
			return true
		}
	}
	return false
}

// Off lists the held-back features that are switched off right now, for the
// web app (sent with the signed-in person's details).
func Off() []string {
	out := []string{}
	for _, n := range Held {
		if !On(n) {
			out = append(out, n)
		}
	}
	return out
}
