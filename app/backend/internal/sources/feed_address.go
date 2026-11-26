package sources

import (
	"net/url"
	"regexp"
	"strings"
)

// feedAddress is a Newznab or Torznab address ready to call. An address
// with no path (https://indexer.example) means its /api, and so do a
// Prowlarr site address (http://prowlarr:9696/5) and a Jackett one
// (.../results/torznab) saved without it by earlier versions.
func feedAddress(raw string) string {
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" {
		return raw
	}
	if u.Path == "" || prowlarrSitePath.MatchString(u.Path) || strings.HasSuffix(u.Path, "/results/torznab") {
		return raw + "/api"
	}
	return raw
}

var prowlarrSitePath = regexp.MustCompile(`^/[0-9]+$`)

// siteFeedPath says whether an address path names one site in Prowlarr
// (/5/api, also under a URL base) or Jackett (/api/v2.0/indexers/x/...).
func siteFeedPath(p string) bool {
	return prowlarrFeedPath.MatchString(p) || strings.Contains(p, "/api/v2.0/indexers/")
}

var prowlarrFeedPath = regexp.MustCompile(`/[0-9]+(/api)?/?$`)
