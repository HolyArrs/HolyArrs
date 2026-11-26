package migrate

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// Edge cases for one Source per site, beyond the first tests.

func torznab(name, base string, cats ...int) Provider {
	cs := make([]any, 0, len(cats))
	for _, c := range cats {
		cs = append(cs, float64(c))
	}
	return Provider{Name: name, Kind: "Torznab", Enabled: true, Priority: 25, Protocol: "torrent", NeedsReentry: []string{"apiKey"},
		Settings: map[string]any{"baseUrl": base, "apiPath": "/api", "categories": cs}}
}

func ready(snaps ...*Snapshot) {
	for _, s := range snaps {
		s.Via = ViaAPI
		s.finish()
	}
}

// The same feed in Sonarr and Radarr with different keys: the apps never
// hand out the key, so it is one Source and one key is asked for. A key
// put in the address itself makes them two addresses, so two Sources.
func TestAdvSameSiteDifferentKeys(t *testing.T) {
	so := &Snapshot{Kind: Sonarr, Indexers: []Provider{torznab("Jack", "http://jackett:9117/api/v2.0/indexers/x/results/torznab/", 5000)}}
	ra := &Snapshot{Kind: Radarr, Indexers: []Provider{torznab("Jack", "http://jackett:9117/api/v2.0/indexers/x/results/torznab/", 2000)}}
	ready(so, ra)
	p := BuildPlan([]Input{{1, so}, {2, ra}}, Choices{})
	if n := len(sourceItems(p)); n != 1 {
		t.Fatalf("%d Sources, want 1", n)
	}
	asks := 0
	for _, n := range p.NeedsInput {
		if strings.Contains(n, `indexer "Jack"`) {
			asks++
		}
	}
	if asks != 1 {
		t.Fatalf("key asked %d times: %v", asks, p.NeedsInput)
	}

	so2 := &Snapshot{Kind: Sonarr, Indexers: []Provider{torznab("A", "http://tracker:80/feed/?passkey=aaa", 5000)}}
	ra2 := &Snapshot{Kind: Radarr, Indexers: []Provider{torznab("B", "http://tracker:80/feed/?passkey=bbb", 2000)}}
	ready(so2, ra2)
	if n := len(sourceItems(BuildPlan([]Input{{1, so2}, {2, ra2}}, Choices{}))); n != 2 {
		t.Fatalf("keys in the address: %d Sources, want 2", n)
	}
}

// Two Sonarr (HD and 4K) with the same feed under other categories: the
// Source should search both sets, not silently keep the first one's.
func TestAdvSameSiteDifferentCategories(t *testing.T) {
	hd := &Snapshot{Kind: Sonarr, Indexers: []Provider{torznab("Jack", "http://jackett:9117/t/", 5030)}}
	uhd := &Snapshot{Kind: Sonarr, Indexers: []Provider{torznab("Jack 4K", "http://jackett:9117/t/", 5045)}}
	ready(hd, uhd)
	items := sourceItems(BuildPlan([]Input{{1, hd}, {2, uhd}}, Choices{}))
	if len(items) != 1 {
		t.Fatalf("%d Sources", len(items))
	}
	got := items[0].Source.Categories["tv"]
	if !slices.Contains(got, 5030) || !slices.Contains(got, 5045) {
		t.Errorf("tv categories %v: the second Sonarr's 5045 was dropped", got)
	}
}

// A Prowlarr copy in Sonarr whose id is not in the imported Prowlarr (it was
// deleted there, or comes from another Prowlarr) is its own Source.
func TestAdvSiteOnlyInSonarr(t *testing.T) {
	pr, so, ra := sameSetup()
	so.Indexers = append(so.Indexers, torznab("Gone (Prowlarr)", "http://prowlarr:9696/42/", 5000))
	ready(so)
	p := BuildPlan([]Input{{1, pr}, {2, so}, {3, ra}}, Choices{})
	if n := len(sourceItems(p)); n != 3 {
		t.Fatalf("%d Sources, want 3", n)
	}
	gone := item(t, p, GroupSources, "source:gone (prowlarr)").Source
	if gone.URL != "http://prowlarr:9696/42/api" || len(gone.SameAs) != 0 {
		t.Fatalf("gone %+v", gone)
	}
	if !slices.Contains(p.NeedsInput, `sonarr: indexer "Gone (Prowlarr)": apiKey`) {
		t.Fatalf("its key is not asked: %v", p.NeedsInput)
	}
}

// Planning the same apps twice gives the same plan, and a second move
// adds nothing.
func TestAdvImportTwice(t *testing.T) {
	pr, so, ra := sameSetup()
	a, _ := json.Marshal(sourceItems(BuildPlan([]Input{{1, pr}, {2, so}, {3, ra}}, Choices{})))
	b, _ := json.Marshal(sourceItems(BuildPlan([]Input{{1, pr}, {2, so}, {3, ra}}, Choices{})))
	if string(a) != string(b) {
		t.Fatalf("plans differ:\n%s\n%s", a, b)
	}
	l := newMoveLab(t)
	l.keep(t, pr, so, ra)
	l.move(t, Choices{})
	before := l.count(t, "SELECT COUNT(*) FROM sources")
	l.move(t, Choices{})
	if after := l.count(t, "SELECT COUNT(*) FROM sources"); after != before {
		t.Fatalf("sources %d -> %d", before, after)
	}
}

// Undo removes the one Source it made per site, and importing again makes
// each one once.
func TestAdvUndoThenAgain(t *testing.T) {
	pr, so, ra := sameSetup()
	l := newMoveLab(t)
	start := l.count(t, "SELECT COUNT(*) FROM sources")
	l.keep(t, pr, so, ra)
	mv := l.move(t, Choices{})
	if got := l.count(t, "SELECT COUNT(*) FROM sources"); got != start+2 {
		t.Fatalf("after move %d, want %d", got, start+2)
	}
	l.undo(t, mv.ID)
	if got := l.count(t, "SELECT COUNT(*) FROM sources"); got != start {
		t.Fatalf("after undo %d, want %d", got, start)
	}
	l.move(t, Choices{})
	if got := l.count(t, "SELECT COUNT(*) FROM sources"); got != start+2 {
		t.Fatalf("after again %d, want %d", got, start+2)
	}
}

// A feed on Prowlarr's host but not Prowlarr (another port, another name)
// stays its own Source, as does a Jackett on the same host and port path.
func TestAdvOtherFeedSameHost(t *testing.T) {
	pr, so, ra := sameSetup()
	so.Indexers = append(so.Indexers,
		torznab("Other", "http://prowlarr:9117/7/", 5000),
		torznab("Jackett on box", "http://10.0.0.5:9117/api/v2.0/indexers/x/results/torznab/", 5000))
	ready(so)
	p := BuildPlan([]Input{{1, pr}, {2, so}, {3, ra}}, Choices{})
	if n := len(sourceItems(p)); n != 4 {
		t.Fatalf("%d Sources, want 4: %+v", n, sourceItems(p))
	}
	item(t, p, GroupSources, "source:other")
	item(t, p, GroupSources, "source:jackett on box")
}
