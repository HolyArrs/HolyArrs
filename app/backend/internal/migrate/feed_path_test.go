package migrate

import "testing"

// An indexer the old app reaches through Prowlarr or Jackett keeps its
// feed path, as the old app calls it: base address plus "/api".
func TestImportedProwlarrSiteKeepsItsFeedPath(t *testing.T) {
	for base, want := range map[string]string{
		"http://192.168.1.36:9696/5/":                              "http://192.168.1.36:9696/5/api",
		"http://192.168.1.36:9696/5/api":                           "http://192.168.1.36:9696/5/api",
		"http://jackett:9117/api/v2.0/indexers/x/results/torznab/": "http://jackett:9117/api/v2.0/indexers/x/results/torznab/api",
	} {
		ix := Provider{ID: 1, Name: "Nyaa.si (Prowlarr)", Kind: "Torznab", Enabled: true, Priority: 25,
			Settings: map[string]any{"baseUrl": base, "apiPath": "/api"}, NeedsReentry: []string{"apiKey"}}
		p := BuildPlan([]Input{{1, &Snapshot{Kind: Sonarr, Indexers: []Provider{ix}}}}, Choices{})
		var got string
		for _, it := range p.Group(GroupSources).Items {
			if it.Source != nil {
				got = it.Source.URL
			}
		}
		if got != want {
			t.Errorf("%s: imported as %q, want %q", base, got, want)
		}
	}
}
