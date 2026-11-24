package plugins

import "testing"

func TestManifestDetails(t *testing.T) {
	m, err := ParseManifest(manifestJSON(func(v map[string]any) {
		v["long_description"] = "Two lines.\nMore."
		v["how_it_works"] = []string{"Install it", "Sign in"}
		v["developers"] = []string{"CoderLuii", "holyarrs-community"}
		v["updated"] = "2026-11-19"
		v["repo"] = "https://github.com/HolyArrs/HolyArrs-Plugins"
		v["platforms"] = []string{"linux-amd64"}
		v["support_email"] = "help@example.org"
		v["support_url"] = "https://example.org/help"
		v["size"] = 4096
		v["some_future_key"] = 1
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Developers) != 2 || m.Updated != "2026-11-19" || len(m.HowItWorks) != 2 || m.SupportEmail != "help@example.org" || m.Size != 4096 {
		t.Fatalf("details not read: %+v", m.Details)
	}
	for name, mod := range map[string]func(map[string]any){
		"markup":    func(v map[string]any) { v["long_description"] = "<b>x</b>" },
		"login":     func(v map[string]any) { v["developers"] = []string{"bad login"} },
		"date":      func(v map[string]any) { v["updated"] = "yesterday" },
		"repo":      func(v map[string]any) { v["repo"] = "javascript:alert(1)" },
		"email":     func(v map[string]any) { v["support_email"] = "not mail" },
		"support":   func(v map[string]any) { v["support_url"] = "http://example.org" },
		"size":      func(v map[string]any) { v["size"] = -1 },
		"emptyStep": func(v map[string]any) { v["how_it_works"] = []string{" "} },
	} {
		if _, err := ParseManifest(manifestJSON(mod)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
