package sites

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

// PluginSite builds the subtitle site a plugin serves (plugin://<id>); the
// app sets it before any Job runs. Nil means plugin sites are not set up
// yet, which a make Job treats as a plugin that is not running.
var PluginSite func(id string) Site

var pluginIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// PluginOf is the plugin id of a plugin site address, plugin://<id> and
// nothing else, or "".
func PluginOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "plugin" || !pluginIDRe.MatchString(u.Host) || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return ""
	}
	return u.Host
}

func newPlugin(raw string) (Site, error) {
	id := PluginOf(raw)
	if id == "" {
		return nil, errors.New("a plugin subtitle site's address is plugin:// and the plugin's id")
	}
	if PluginSite == nil {
		return nil, fmt.Errorf("%w: plugin subtitle sites are not set up yet", ErrPluginGone)
	}
	return PluginSite(id), nil
}

// ErrPluginGone is a plugin site that is not running or was stopped
// mid-call. A Job that meets it is tried again once the plugin is back.
var ErrPluginGone = errors.New("the plugin is not running")
