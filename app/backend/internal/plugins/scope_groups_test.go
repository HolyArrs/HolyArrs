package plugins

import (
	"reflect"
	"testing"
)

func TestScopeGroups(t *testing.T) {
	for _, c := range []struct {
		name    string
		actions []string
		want    []ScopeGroup
	}{
		{"none", nil, nil},
		{"read only", []string{"library.read"}, []ScopeGroup{{ID: "library", Scopes: []string{"library.read"}}}},
		{"change and read", []string{"library.read", "library.change"}, []ScopeGroup{{ID: "library", Change: true, Scopes: []string{"library.change", "library.read"}}}},
		{"red lines", []string{"network.forward", "files.rename.change", "library.delete"}, []ScopeGroup{
			{ID: "library", Change: true, Scopes: []string{"library.delete"}, Warnings: []string{"delete"}},
			{ID: "files", Change: true, Scopes: []string{"files.rename.change"}, Warnings: []string{"files"}},
			{ID: "network", Change: true, Scopes: []string{"network.forward"}, Warnings: []string{"forward"}},
		}},
		{"artwork", []string{"library.artwork.set"}, []ScopeGroup{{ID: "library", Change: true, Scopes: []string{"library.artwork.set"}, Warnings: []string{"artwork"}}}},
		{"messages", []string{"alerts.webhooks.send"}, []ScopeGroup{{ID: "alerts", Change: true, Scopes: []string{"alerts.webhooks.send"}, Warnings: []string{"notify"}}}},
		{"ratings", []string{"sources.metadata"}, []ScopeGroup{{ID: "network", Change: true, Scopes: []string{"sources.metadata"}, Warnings: []string{"ratings"}}}},
		{"title sections", []string{"pages.title-section"}, []ScopeGroup{{ID: "network", Change: true, Scopes: []string{"pages.title-section"}, Warnings: []string{"sections"}}}},
		{"downloader", []string{"sources.downloader"}, []ScopeGroup{{ID: "network", Change: true, Scopes: []string{"sources.downloader"}, Warnings: []string{"downloader"}}}},
	} {
		if got := scopeGroups(&Access{Actions: c.actions}); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}
