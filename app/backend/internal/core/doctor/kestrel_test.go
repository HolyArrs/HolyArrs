package doctor

import (
	"net/http"
	"strings"
	"testing"
)

// The real answers of the lab apps on the paths HolyArrs calls: Sonarr
// answers /api/v2/app/version and /api/v3/system/status with an empty 401,
// Jellyfin answers /api/v2/app/version, /sabnzbd/api and /identity with an
// empty 404. Neither names itself; both carry "Server: Kestrel".
func TestOtherKnowsAnEmptyKestrelAnswer(t *testing.T) {
	sonarr401 := http.Header{"Content-Length": {"0"}, "Server": {"Kestrel"}}
	jellyfin404 := http.Header{"Content-Length": {"0"}, "Server": {"Kestrel"}, "Content-Language": {"en-US"}, "X-Response-Time-Ms": {"24.6864"}}
	for _, tc := range []struct {
		name string
		h    http.Header
		own  []string
		want bool
	}{
		{"qbit at sonarr", sonarr401, []string{"qbittorrent"}, true},
		{"sab at sonarr", sonarr401, []string{"sabnzbd"}, true},
		{"plex at jellyfin", jellyfin404, []string{"plex"}, true},
		{"qbit at jellyfin", jellyfin404, []string{"qbittorrent"}, true},
		{"jellyfin at jellyfin", jellyfin404, []string{"jellyfin", "emby"}, false},
		{"emby at jellyfin", jellyfin404, []string{"emby", "jellyfin"}, false},
		{"source at prowlarr", sonarr401, []string{"prowlarr", "jackett", "nzbhydra"}, false},
		{"servarr version header", http.Header{"X-Application-Version": {"4.0.20"}}, []string{"sabnzbd"}, true},
		{"bare 404, no server", http.Header{"Content-Length": {"0"}}, []string{"plex"}, false},
	} {
		got := Other(tc.h, nil, tc.own...) != nil
		if got != tc.want {
			t.Errorf("%s: Other = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNotFoundNamesNoOtherApp(t *testing.T) {
	if strings.Contains(Line(NotFound), "sabnzbd") {
		t.Fatalf("NotFound line names /sabnzbd for every kind: %q", Line(NotFound))
	}
}

// LineOf rebuilds a stored line, keeping the missing category's name.
func TestLineOfKeepsTheCategoryName(t *testing.T) {
	for msg, want := range map[string]string{
		CategoryLine("tv-hd"): CategoryLine("tv-hd"),
		"the category does not exist in SABnzbd; add it there first (Config > Categories) (tv-hd)": CategoryLine("tv-hd"),
		Line(UseHTTPS):       Line(UseHTTPS),
		"sabnzbd: EOF":       "",
		"connection refused": Line(Refused),
	} {
		if got := LineOf(msg); got != want {
			t.Errorf("LineOf(%q) = %q, want %q", msg, got, want)
		}
	}
}
