package sources

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

var pluginHostRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// PluginTransport carries a plugin search source's calls (plugin://<id>);
// the app sets it. Nil means plugin sources are not available.
var PluginTransport func(id string) http.RoundTripper

// pluginSource reads the plugin id of a plugin source address.
func pluginSource(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "plugin" || !pluginHostRe.MatchString(u.Host) {
		return "", false
	}
	return u.Host, true
}

// IsPluginSource says whether a Source is served by a plugin.
func IsPluginSource(raw string) bool {
	_, ok := pluginSource(raw)
	return ok
}

func pluginTransport(id string) http.RoundTripper {
	if PluginTransport == nil {
		return refuseTransport{}
	}
	return PluginTransport(id)
}

type refuseTransport struct{}

func (refuseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("plugin sources are not available")
}
