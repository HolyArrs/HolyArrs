// Package hooks lets reviewed plugins provide parts of HolyArrs: a download
// client, a search source, assistant tools, pages, Home widgets, title page
// sections, events, user names and media streams.
//
// A plugin never listens: the net lock keeps it off every port. It asks
// HolyArrs for work with its plugin token (GET .../{hook}/next, long-polled)
// and sends the answer back (POST .../{hook}/{job}). HolyArrs only hands a
// plugin work for a hook whose scope the owner granted, and drops all of a
// plugin's work the moment its token ends.
package hooks

// Hook names, as they appear in the plugin-facing routes.
const (
	Downloader = "downloader"
	Releases   = "releases"
	Tools      = "tools"
	Events     = "events"
	Sections   = "sections"
	Pages      = "pages"
)

// Scopes are the plugin scopes each provider hook needs.
var Scopes = map[string]string{
	Downloader: "sources.downloader",
	Releases:   "sources.releases",
	Tools:      "assistant.tools",
	Events:     "alerts.events.subscribe",
	Sections:   "pages.title-section",
	Pages:      "pages.add",
}
