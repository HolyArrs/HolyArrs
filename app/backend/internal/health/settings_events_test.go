package health

import "testing"

func TestSettingsChangeAreas(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		method, path, area, action string
		ok                         bool
	}{
		{"PUT", "/api/v1/settings/search", "search", "changed", true},
		{"PUT", "/api/v1/auth/settings", "sign-in", "changed", true},
		{"GET", "/api/v1/auth/settings", "", "", false},
		{"POST", "/api/v1/pick-rules", "pick-rules", "added", true},
		{"PUT", "/api/v1/library-folders/3", "library-folders", "changed", true},
		{"PUT", "/api/v1/library-folders/3/path", "library-folders", "changed", true},
		{"PUT", "/api/v1/downloaders/3/path", "", "", false},
		{"DELETE", "/api/v1/sources/4", "sources", "deleted", true},
		{"PUT", "/api/v1/outside-lists/5", "outside-lists", "changed", true},
		{"POST", "/api/v1/downloaders", "downloaders", "added", true},
		{"POST", "/api/v1/downloaders/2/pause", "", "", false},
		{"POST", "/api/v1/sources/check", "", "", false},
		{"POST", "/api/v1/settings/naming/example", "", "", false},
		{"GET", "/api/v1/settings/search", "", "", false},
		{"PUT", "/api/v1/titles/1", "", "", false},
	} {
		area, _, action, _, ok := SettingsChange(c.method, c.path)
		if ok != c.ok || area != c.area || action != c.action {
			t.Errorf("%s %s: %q %q %v", c.method, c.path, area, action, ok)
		}
	}
}
