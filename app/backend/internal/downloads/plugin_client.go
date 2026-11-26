package downloads

import (
	"regexp"
	"strings"
)

var pluginIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// PluginAdapter says whether adapter is a download client a plugin serves:
// plugin-torrent, plugin-usenet, or plugin for a download type the plugin
// brings itself.
func PluginAdapter(adapter string) bool {
	return adapter == "plugin-torrent" || adapter == "plugin-usenet" || adapter == "plugin"
}

// PluginID reads the plugin id from a plugin download client's address,
// plugin://<id>.
func PluginID(raw string) (string, bool) {
	id, ok := strings.CutPrefix(raw, "plugin://")
	return id, ok && pluginIDRe.MatchString(id)
}
