package downloads

import (
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestPluginClientAddress(t *testing.T) {
	for _, c := range []struct {
		raw string
		id  string
		ok  bool
	}{
		{"plugin://my-client", "my-client", true},
		{"plugin://", "", false},
		{"plugin://../x", "", false},
		{"plugin://a/b", "", false},
		{"http://my-client", "", false},
		{"plugin://UPPER", "", false},
	} {
		id, ok := PluginID(c.raw)
		if ok != c.ok || ok && id != c.id {
			t.Errorf("%q: %q %v", c.raw, id, ok)
		}
	}
	for _, c := range []struct {
		adapter, url string
		protocol     string
		ok           bool
	}{
		{"plugin-torrent", "plugin://dl", "torrent", true},
		{"plugin-usenet", "plugin://dl", "usenet", true},
		{"plugin-torrent", "http://10.0.0.2:8080", "", false},
		{"qbittorrent", "plugin://dl", "", false},
	} {
		var d domain.Downloader
		err := fill(Input{Name: "Plugin", Adapter: c.adapter, URL: c.url}, &d, true)
		if (err == nil) != c.ok || c.ok && (d.Protocol != c.protocol || d.URL != c.url) {
			t.Errorf("%s %s: %v %+v", c.adapter, c.url, err, d)
		}
	}
	m := &Manager{}
	if _, err := m.engineWith(domain.Downloader{Adapter: "plugin-torrent", URL: "plugin://dl"}, ""); err == nil {
		t.Error("a plugin client was built without plugin support")
	}
	var asked string
	m.PluginEngine = func(id string) Engine { asked = id; return nil }
	if _, err := m.engineWith(domain.Downloader{Adapter: "plugin-torrent", URL: "plugin://dl"}, ""); err != nil || asked != "dl" {
		t.Errorf("engine for plugin: %v %q", err, asked)
	}
}
