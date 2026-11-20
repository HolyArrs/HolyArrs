package migrate

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// sameSetup is Prowlarr with three indexers synced into Sonarr and Radarr:
// Nyaa.si (a site HolyArrs has built in), MyJackett (a Torznab feed in
// Prowlarr) and OneJAV (a site HolyArrs reads only through Prowlarr).
// Prowlarr was read at another address than the one it gives the apps, and
// Radarr reaches it under a third host name (a VPN container).
func sameSetup() (prowlarr, sonarr, radarr *Snapshot) {
	prowlarr = &Snapshot{Kind: Prowlarr, Version: "2.6.5", Address: "http://10.0.0.5:9696",
		AppLinks: []Provider{{Name: "Sonarr", Kind: "Sonarr", Settings: map[string]any{"prowlarrUrl": "http://prowlarr:9696", "baseUrl": "http://sonarr:8989"}}},
		Indexers: []Provider{
			{ID: 3, Name: "Nyaa.si", Kind: "Cardigann", Enabled: true, Priority: 25, Protocol: "torrent", Settings: map[string]any{"definitionFile": "nyaasi"}},
			{ID: 7, Name: "MyJackett", Kind: "Torznab", Enabled: true, Priority: 25, Protocol: "torrent",
				Settings: map[string]any{"baseUrl": "http://jackett:9117/api/v2.0/indexers/x/results/torznab/"}, NeedsReentry: []string{"apiKey"}},
			{ID: 9, Name: "OneJAV", Kind: "Cardigann", Enabled: true, Priority: 25, Protocol: "torrent", Settings: map[string]any{"definitionFile": "onejav"}},
		}}
	synced := func(host string, id, cat int, name string) Provider {
		return Provider{Name: name + " (Prowlarr)", Kind: "Torznab", Enabled: true, Priority: 25, Protocol: "torrent", NeedsReentry: []string{"apiKey"},
			Settings: map[string]any{"baseUrl": "http://" + host + ":9696/" + strconv.Itoa(id) + "/", "apiPath": "/api", "categories": []any{float64(cat)}}}
	}
	sonarr = &Snapshot{Kind: Sonarr, Version: "4.0.0", Indexers: []Provider{
		synced("prowlarr", 3, 5070, "Nyaa.si"), synced("prowlarr", 7, 5000, "MyJackett"), synced("prowlarr", 9, 5000, "OneJAV")}}
	radarr = &Snapshot{Kind: Radarr, Version: "6.0.0", Indexers: []Provider{
		synced("gluetun", 3, 2000, "Nyaa.si"), synced("prowlarr", 7, 2000, "MyJackett"), synced("prowlarr", 9, 2000, "OneJAV")}}
	for _, s := range []*Snapshot{prowlarr, sonarr, radarr} {
		s.Via = ViaAPI
		s.finish()
	}
	return prowlarr, sonarr, radarr
}

func sourceItems(p *Plan) []Item {
	for _, g := range p.Groups {
		if g.Name == GroupSources {
			return g.Items
		}
	}
	return nil
}

// TestProwlarrAndArrsAreOneSource: Prowlarr, Sonarr and Radarr imported
// together bring each site once, whichever order they are given in.
func TestProwlarrAndArrsAreOneSource(t *testing.T) {
	pr, so, ra := sameSetup()
	var first []byte
	for _, in := range [][]Input{{{1, pr}, {2, so}, {3, ra}}, {{2, so}, {3, ra}, {1, pr}}, {{2, so}, {1, pr}, {3, ra}}} {
		p := BuildPlan(in, Choices{})
		items := sourceItems(p)
		if len(items) != 3 {
			t.Fatalf("%d Sources: %+v", len(items), items)
		}
		nyaa := item(t, p, GroupSources, "source:nyaa.si")
		if sp := nyaa.Source; sp.Format != "site" || sp.Site != "nyaasi" || sp.URL != "https://nyaa.si" || len(sp.SameAs) != 2 {
			t.Fatalf("nyaa %+v", sp)
		}
		if !slices.Contains(nyaa.LegacyKeys, "source:nyaa.si (prowlarr)") || !strings.HasPrefix(nyaa.From, "prowlarr ") {
			t.Fatalf("nyaa %+v", nyaa)
		}
		// Radarr reaches Prowlarr as gluetun: the same id and Prowlarr's name make it the same.
		if got := nyaa.Source.SameAs; got[0].App != "sonarr" || got[1].App != "radarr" || got[1].URL != "http://gluetun:9696/3/api" || got[1].Name != "Nyaa.si (Prowlarr)" {
			t.Fatalf("same as %+v", got)
		}
		if !slices.Contains(nyaa.Source.Through, "http://prowlarr:9696/3/api") || !slices.Contains(nyaa.Source.Through, "http://10.0.0.5:9696/3/api") {
			t.Fatalf("through %v", nyaa.Source.Through)
		}
		jk := item(t, p, GroupSources, "source:myjackett").Source
		if jk.Format != "torznab" || !strings.HasPrefix(jk.URL, "http://jackett:9117/") || !slices.Equal(jk.Categories["tv"], []int{5000}) ||
			!slices.Equal(jk.Categories["movie"], []int{2000}) || len(jk.SameAs) != 2 {
			t.Fatalf("jackett %+v", jk)
		}
		// Prowlarr cannot give OneJAV: the copy through Prowlarr comes over once, for shows and movies.
		oj := item(t, p, GroupSources, "source:onejav (prowlarr)").Source
		if oj.URL != "http://prowlarr:9696/9/api" || !slices.Equal(oj.Kinds, []string{"tv", "movie"}) || !slices.Equal(oj.Categories["movie"], []int{2000}) || len(oj.SameAs) != 1 {
			t.Fatalf("onejav %+v", oj)
		}
		for _, f := range []string{"sonarr indexers[0]", "radarr indexers[0]", "radarr indexers[2]"} {
			if n, ok := notMoved(p, f); ok {
				t.Fatalf("%s listed as not moved: %+v", f, n)
			}
		}
		// No key is asked for an indexer that went into another Source.
		for _, n := range p.NeedsInput {
			if strings.Contains(n, "Nyaa.si") || strings.Contains(n, "MyJackett (Prowlarr)") || strings.HasPrefix(n, "radarr: indexer \"OneJAV") {
				t.Fatalf("needs %v", p.NeedsInput)
			}
		}
		if !slices.Contains(p.NeedsInput, `sonarr: indexer "OneJAV (Prowlarr)": apiKey`) {
			t.Fatalf("needs %v", p.NeedsInput)
		}
		// The blocklist keeps a release under the Source of the old indexer's name.
		for name, want := range map[string]string{"Nyaa.si (Prowlarr)": "Nyaa.si", "nyaa.si": "Nyaa.si", "MyJackett (Prowlarr)": "MyJackett",
			"OneJAV (Prowlarr)": "OneJAV (Prowlarr)", "Other": ""} {
			if got := p.SourceFor(name); got != want {
				t.Fatalf("SourceFor(%q) = %q, want %q", name, got, want)
			}
		}
		got, _ := json.Marshal(items)
		if first == nil {
			first = got
		} else if string(got) != string(first) {
			t.Fatalf("another order planned other Sources:\n%s\n%s", first, got)
		}
	}
}

// TestProwlarrAndArrsRadarrFirst: with Radarr read before Sonarr the same
// three Sources are planned; only the order of the apps they list differs.
func TestProwlarrAndArrsRadarrFirst(t *testing.T) {
	pr, so, ra := sameSetup()
	p := BuildPlan([]Input{{3, ra}, {2, so}, {1, pr}}, Choices{})
	if items := sourceItems(p); len(items) != 3 {
		t.Fatalf("%d Sources", len(items))
	}
	sa := item(t, p, GroupSources, "source:nyaa.si").Source.SameAs
	if len(sa) != 2 || sa[0].App != "radarr" || sa[1].App != "sonarr" {
		t.Fatalf("same as %+v", sa)
	}
	if p.SourceFor("OneJAV (Prowlarr)") != "OneJAV (Prowlarr)" {
		t.Fatal("OneJAV not found")
	}
}

// TestSameFeedInSonarrAndRadarr: one feed under two names in Sonarr and
// Radarr is one Source with both kinds' categories.
func TestSameFeedInSonarrAndRadarr(t *testing.T) {
	so := &Snapshot{Kind: Sonarr, Indexers: []Provider{{Name: "Nyaa", Kind: "Torznab", Enabled: true, Priority: 25,
		Settings: map[string]any{"baseUrl": "http://Jackett:9117/api/v2.0/indexers/nyaa/results/torznab/", "categories": []any{float64(5000)}}}}}
	ra := &Snapshot{Kind: Radarr, Indexers: []Provider{{Name: "nyaa.si", Kind: "Torznab", Enabled: false, Priority: 10,
		Settings: map[string]any{"baseUrl": "http://jackett:9117/api/v2.0/indexers/nyaa/results/torznab", "categories": []any{float64(2000)}, "minimumSeeders": float64(3)}}}}
	for _, in := range [][]Input{{{1, so}, {2, ra}}, {{2, ra}, {1, so}}} {
		p := BuildPlan(in, Choices{})
		if items := sourceItems(p); len(items) != 1 {
			t.Fatalf("%d Sources", len(items))
		}
		it := sourceItems(p)[0]
		sp := it.Source
		if !slices.Equal(sp.Kinds, []string{"tv", "movie"}) && !slices.Equal(sp.Kinds, []string{"movie", "tv"}) {
			t.Fatalf("kinds %v", sp.Kinds)
		}
		if !slices.Equal(sp.Categories["tv"], []int{5000}) || !slices.Equal(sp.Categories["movie"], []int{2000}) || !sp.On || sp.MinSeeders == nil || *sp.MinSeeders != 3 || len(sp.SameAs) != 1 {
			t.Fatalf("source %+v", sp)
		}
		other := map[string]string{"Nyaa": "nyaa.si", "nyaa.si": "Nyaa"}[it.Name]
		if p.SourceFor(other) != it.Name {
			t.Fatalf("SourceFor(%q) = %q", other, p.SourceFor(other))
		}
	}
}

// keep saves Snapshots in the lab and makes them the ones a move reads.
func (l *moveLab) keep(t *testing.T, snaps ...*Snapshot) {
	t.Helper()
	l.snaps = nil
	for _, s := range snaps {
		saved, err := Save(context.Background(), l.st, 1, s)
		if err != nil {
			t.Fatal(err)
		}
		l.snaps = append(l.snaps, saved.ID)
	}
}

func (l *moveLab) sourceNames(t *testing.T) []string {
	t.Helper()
	rows, err := l.st.Read().Query("SELECT name FROM sources ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// TestProwlarrAndArrsAcrossImports: whether Prowlarr is imported with
// Sonarr and Radarr, before them or after them, each site is added once,
// and importing again adds nothing.
func TestProwlarrAndArrsAcrossImports(t *testing.T) {
	pr, so, ra := sameSetup()
	for _, tc := range []struct {
		name  string
		moves [][]*Snapshot
		want  []string
	}{
		{"together", [][]*Snapshot{{pr, so, ra}, {pr, so, ra}}, []string{"MyJackett", "Nyaa.si", "OneJAV (Prowlarr)"}},
		{"arrs first", [][]*Snapshot{{so, ra}, {pr}, {pr, so, ra}}, []string{"MyJackett (Prowlarr)", "Nyaa.si (Prowlarr)", "OneJAV (Prowlarr)"}},
		{"prowlarr first", [][]*Snapshot{{pr}, {so, ra}, {pr, so, ra}}, []string{"MyJackett", "Nyaa.si", "OneJAV (Prowlarr)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := newMoveLab(t)
			for i, snaps := range tc.moves {
				l.keep(t, snaps...)
				mv := l.move(t, Choices{})
				if mv.State != "done" {
					t.Fatalf("move %d: %+v", i, mv)
				}
				if i < len(tc.moves)-1 {
					continue
				}
				// Importing again adds nothing: each Source line is already here.
				for _, ln := range mv.Report.Lines {
					if ln.Group == GroupSources && ln.Result != ResAlready {
						t.Fatalf("move %d: %+v", i, ln)
					}
				}
			}
			if got := l.sourceNames(t); !slices.Equal(got, tc.want) {
				t.Fatalf("sources %v, want %v", got, tc.want)
			}
			// Importing Prowlarr alone once more finds every one of its Sources here.
			l.keep(t, pr)
			for _, ln := range l.move(t, Choices{}).Report.Lines {
				if ln.Group == GroupSources && ln.Result != ResAlready {
					t.Fatalf("prowlarr again: %+v", ln)
				}
			}
			if got := l.sourceNames(t); !slices.Equal(got, tc.want) {
				t.Fatalf("sources %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSameSourceLine: a Source that is here under another name says which.
func TestSameSourceLine(t *testing.T) {
	pr, so, ra := sameSetup()
	l := newMoveLab(t)
	l.keep(t, so, ra)
	l.move(t, Choices{})
	l.keep(t, pr)
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "source:nyaa.si"); got.Result != ResAlready || got.Detail != "the same as Nyaa.si (Prowlarr), a Source that is here" {
		t.Fatalf("line %+v", got)
	}
}
