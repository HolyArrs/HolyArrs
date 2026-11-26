package sources

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
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

// pluginTypeRe is the shape of a download type a plugin brings, such as
// "soulseek"; torrent and usenet are never one.
var pluginTypeRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// PluginTypeName says whether t is shaped like a plugin's own download type.
func PluginTypeName(t string) bool {
	return t != "torrent" && t != "usenet" && pluginTypeRe.MatchString(t)
}

// PluginDownloadTypes lists the download types plugin id declares; the app
// sets it. Nil means no plugin brings any.
var PluginDownloadTypes func(id string) []string

// PluginTypeOK says whether plugin id declares download type t.
func PluginTypeOK(id, t string) bool {
	return PluginTypeName(t) && PluginDownloadTypes != nil && slices.Contains(PluginDownloadTypes(id), t)
}

// pluginTypeFits says whether a Source at raw may carry download type t: a
// plugin Source whose plugin declares it.
func pluginTypeFits(raw, t string) bool {
	id, ok := pluginSource(raw)
	return ok && PluginTypeOK(id, t)
}

// addressProto is the stored protocol when an edit gives a new address, so a
// plugin type is checked again against it; "" otherwise.
func addressProto(in Input, src *domain.Source) string {
	if strings.TrimSpace(in.URL) == "" {
		return ""
	}
	return src.Protocol
}
