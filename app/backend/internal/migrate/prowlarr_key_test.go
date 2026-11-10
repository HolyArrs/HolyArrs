package migrate

import (
	"context"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// A Source going through the Prowlarr read with its key gets that key; a
// Source at a site's own address gets none.
func TestMovedSourceThroughProwlarrGetsItsKey(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Indexers = []Provider{
			{Name: "Nyaa", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/1/"}, NeedsReentry: []string{"apiKey"}},
			{Name: "Site", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://site.example:9696/1/"}, NeedsReentry: []string{"apiKey"}},
		}
	})
	ctx := context.Background()
	v, err := secrets.Open(ctx, l.st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	l.mv.Secrets = v
	const key = "prowlarr-test-key"
	c := secretsfile.MoveCredential{Address: "http://Prowlarr:9696", APIKey: key}
	if err := secretsfile.SaveMoveCredential(ctx, l.st, v, string(Prowlarr), c, domain.System); err != nil {
		t.Fatal(err)
	}
	if mv := l.move(t, Choices{}); mv.State != "done" {
		t.Fatalf("move %s", mv.State)
	}
	ref := func(name string) *int64 {
		var r *int64
		if err := l.st.Read().QueryRow("SELECT secret_ref FROM sources WHERE name = ?", name).Scan(&r); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return r
	}
	r := ref("Nyaa")
	if r == nil {
		t.Fatal("the Source through Prowlarr has no key")
	}
	if got, err := v.Get(ctx, *r); err != nil || got != key {
		t.Fatal("the Source through Prowlarr does not hold Prowlarr's key")
	}
	if ref("Site") != nil {
		t.Fatal("a Source at another address got Prowlarr's key")
	}
}

// A backup holds no key: a Source through Prowlarr says it needs Prowlarr's.
func TestBackupSourceThroughProwlarrNeedsKey(t *testing.T) {
	ix := Provider{ID: 1, Name: "Nyaa", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/5/"}}
	site := Provider{ID: 2, Name: "Site", Kind: "Torznab", Enabled: true, Priority: 25, Settings: map[string]any{"baseUrl": "http://site.example/torznab"}}
	p := BuildPlan([]Input{{1, &Snapshot{Kind: Sonarr, Via: ViaBackup, Indexers: []Provider{ix, site}}}}, Choices{})
	seen := 0
	for _, it := range p.Group(GroupSources).Items {
		if it.Source == nil {
			continue
		}
		seen++
		says := strings.Contains(it.Needs, "Prowlarr's API key") && it.Source.NeedsKey
		if it.Name == "Nyaa" && !says {
			t.Errorf("Nyaa: needs %q, detail %q", it.Needs, it.Detail)
		}
		if it.Name == "Site" && strings.Contains(it.Detail, "Prowlarr") {
			t.Errorf("Site mentions Prowlarr: %q", it.Detail)
		}
	}
	if seen != 2 {
		t.Fatalf("%d Sources planned, want 2", seen)
	}
}
