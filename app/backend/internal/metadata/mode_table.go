package metadata

import "sort"

// SourceRow is one kind of detail and where it comes from in each mode. The
// wiring and the Settings answer both read this table, so the "Off on this
// install" list says exactly what the code does.
type SourceRow struct {
	ID   string
	Name string
	// Server: the metadata server gives it in HolyArrs server mode.
	Server bool
	// Own: asked directly in Your own keys mode; Key, when set, is the own
	// key it needs. Everywhere: asked directly in every mode but off, once
	// Key is set (TheTVDB).
	Own        bool
	Key        string
	Everywhere bool
	// Hosts are the outside addresses asked directly for it.
	Hosts []string
}

// ServerHosts are the metadata server's addresses (the old one answers
// pictures saved under it until version 2).
var ServerHosts = []string{"api.holyarrs.com", "holyarrs-api.coderluii.dev"}

// SourceTable lists every kind of detail. Charts, Apple rows, streaming
// Top 10s, Wikipedia facts and anime maps come only from the metadata
// server: no direct client exists for them in any mode.
var SourceTable = []SourceRow{
	{ID: "tmdb", Name: "Movie and show details, search and Explore", Server: true, Own: true, Key: KeyTMDB, Hosts: []string{"api.themoviedb.org", "image.tmdb.org"}},
	{ID: "tvmaze", Name: "TVmaze show details and air times", Server: true, Own: true, Hosts: []string{"api.tvmaze.com", "static.tvmaze.com"}},
	{ID: "tvdb", Name: "TheTVDB show details", Everywhere: true, Key: KeyTVDB, Hosts: []string{"api4.thetvdb.com", "artworks.thetvdb.com"}},
	{ID: "music", Name: "Artists, albums and artist pictures", Server: true, Own: true, Hosts: []string{"musicbrainz.org", "api.deezer.com", "en.wikipedia.org"}},
	{ID: "trailers", Name: "Trailers", Server: true, Own: true, Key: KeyTMDB, Hosts: []string{"www.youtube.com"}},
	{ID: "ratings", Name: "Outside ratings", Server: true, Own: true, Key: KeyMDBList, Hosts: []string{"api.mdblist.com"}},
	{ID: "art", Name: "Title logos and wide backgrounds", Server: true, Own: true, Key: KeyFanart, Hosts: []string{"webservice.fanart.tv", "assets.fanart.tv"}},
	{ID: "charts", Name: "Music charts and new music rows", Server: true},
	{ID: "streaming", Name: "Top 10 on streaming services", Server: true},
	{ID: "wiki", Name: "Wikipedia facts and awards", Server: true},
	{ID: "anime", Name: "Anime names and episode numbers", Server: true},
}

// SourceState is one row of the Settings answer: on, off, or needs_key
// (Your own keys mode, the key is not set yet).
type SourceState struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Key   string `json:"key,omitempty"`
}

// state is one row's state in mode; has says an own key is set.
func (r SourceRow) state(mode Mode, has func(string) bool) string {
	keyed := r.Key == "" || has(r.Key)
	switch {
	case mode == ModeOff:
		return "off"
	case r.Everywhere && keyed:
		return "on"
	case r.Everywhere:
		return "needs_key"
	case mode == ModeServer && r.Server:
		return "on"
	case mode == ModeOwn && r.Own && keyed:
		return "on"
	case mode == ModeOwn && r.Own:
		return "needs_key"
	}
	return "off"
}

// SourceStates is the table's state in mode.
func SourceStates(mode Mode, has func(string) bool) []SourceState {
	out := make([]SourceState, 0, len(SourceTable))
	for _, r := range SourceTable {
		out = append(out, SourceState{ID: r.ID, Name: r.Name, State: r.state(mode, has), Key: r.Key})
	}
	return out
}

// SourceOn says the row id works in mode.
func SourceOn(id string, mode Mode, has func(string) bool) bool {
	for _, r := range SourceTable {
		if r.ID == id {
			return r.state(mode, has) == "on"
		}
	}
	return false
}

// ModeHosts are the outside addresses lookups use in mode, for a firewall
// that blocks by default (trailers' video servers and your own services
// are not lookups and are listed in the docs).
func ModeHosts(mode Mode, has func(string) bool) []string {
	seen := map[string]bool{}
	if mode == ModeServer {
		for _, h := range ServerHosts {
			seen[h] = true
		}
	}
	for _, r := range SourceTable {
		direct := r.Everywhere || mode == ModeOwn && r.Own || mode == ModeServer && r.ID == "trailers" || mode == ModeServer && r.ID == "tmdb"
		if r.state(mode, has) != "on" || !direct {
			continue
		}
		hosts := r.Hosts
		if mode == ModeServer && r.ID == "tmdb" {
			hosts = []string{"image.tmdb.org"} // posters are saved from TMDB itself
		}
		for _, h := range hosts {
			seen[h] = true
		}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
