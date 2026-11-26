package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// sameSetup is Prowlarr with two indexers synced into Sonarr and Radarr:
// MyJackett (a Torznab feed in Prowlarr) and Nyaa.si (a site definition,
// which HolyArrs reads only through Prowlarr). Prowlarr was read at another
// address than the one it gives the apps, and Radarr reaches MyJackett under
// a third host name (a VPN container).
func sameSetup() (prowlarr, sonarr, radarr *Snapshot) {
	prowlarr = &Snapshot{Kind: Prowlarr, Version: "2.6.5", Address: "http://10.0.0.5:9696",
		AppLinks: []Provider{{Name: "Sonarr", Kind: "Sonarr", Settings: map[string]any{"prowlarrUrl": "http://prowlarr:9696", "baseUrl": "http://sonarr:8989"}}},
		Indexers: []Provider{
			{ID: 3, Name: "Nyaa.si", Kind: "Cardigann", Enabled: true, Priority: 25, Protocol: "torrent", Settings: map[string]any{"definitionFile": "nyaasi"}},
			{ID: 7, Name: "MyJackett", Kind: "Torznab", Enabled: true, Priority: 25, Protocol: "torrent",
				Settings: map[string]any{"baseUrl": "http://jackett:9117/api/v2.0/indexers/x/results/torznab/"}, NeedsReentry: []string{"apiKey"}},
		}}
	synced := func(host string, id, cat int, name string) Provider {
		return Provider{Name: name + " (Prowlarr)", Kind: "Torznab", Enabled: true, Priority: 25, Protocol: "torrent", NeedsReentry: []string{"apiKey"},
			Settings: map[string]any{"baseUrl": "http://" + host + ":9696/" + strconv.Itoa(id) + "/", "apiPath": "/api", "categories": []any{float64(cat)}}}
	}
	sonarr = &Snapshot{Kind: Sonarr, Version: "4.0.0", Indexers: []Provider{
		synced("prowlarr", 3, 5070, "Nyaa.si"), synced("prowlarr", 7, 5000, "MyJackett")}}
	radarr = &Snapshot{Kind: Radarr, Version: "6.0.0", Indexers: []Provider{
		synced("prowlarr", 3, 2000, "Nyaa.si"), synced("gluetun", 7, 2000, "MyJackett")}}
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
		if len(items) != 2 {
			t.Fatalf("%d Sources: %+v", len(items), items)
		}
		jackett := item(t, p, GroupSources, "source:myjackett")
		jk := jackett.Source
		if jk.Format != "torznab" || !strings.HasPrefix(jk.URL, "http://jackett:9117/") || !slices.Equal(jk.Categories["tv"], []int{5000}) ||
			!slices.Equal(jk.Categories["movie"], []int{2000}) || len(jk.SameAs) != 2 {
			t.Fatalf("jackett %+v", jk)
		}
		if !slices.Contains(jackett.LegacyKeys, "source:myjackett (prowlarr)") || !strings.HasPrefix(jackett.From, "prowlarr ") {
			t.Fatalf("jackett %+v", jackett)
		}
		// Radarr reaches Prowlarr as gluetun: the same id and Prowlarr's name make it the same.
		if got := jk.SameAs; got[0].App != "sonarr" || got[1].App != "radarr" || got[1].URL != "http://gluetun:9696/7/api" || got[1].Name != "MyJackett (Prowlarr)" {
			t.Fatalf("same as %+v", got)
		}
		if !slices.Contains(jk.Through, "http://prowlarr:9696/7/api") || !slices.Contains(jk.Through, "http://10.0.0.5:9696/7/api") {
			t.Fatalf("through %v", jk.Through)
		}
		// Prowlarr cannot give Nyaa.si (a site definition): the copy through Prowlarr comes over once, for shows and movies.
		ny := item(t, p, GroupSources, "source:nyaa.si (prowlarr)").Source
		if ny.URL != "http://prowlarr:9696/3/api" || !slices.Equal(ny.Kinds, []string{"tv", "movie"}) || !slices.Equal(ny.Categories["movie"], []int{2000}) || len(ny.SameAs) != 1 {
			t.Fatalf("nyaa %+v", ny)
		}
		for _, f := range []string{"sonarr indexers[1]", "radarr indexers[0]", "radarr indexers[1]"} {
			if n, ok := notMoved(p, f); ok {
				t.Fatalf("%s listed as not moved: %+v", f, n)
			}
		}
		// No key is asked for an indexer that went into another Source.
		for _, n := range p.NeedsInput {
			if strings.Contains(n, "MyJackett (Prowlarr)") || strings.HasPrefix(n, "radarr: indexer \"Nyaa.si") {
				t.Fatalf("needs %v", p.NeedsInput)
			}
		}
		if !slices.Contains(p.NeedsInput, `sonarr: indexer "Nyaa.si (Prowlarr)": apiKey`) {
			t.Fatalf("needs %v", p.NeedsInput)
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
// two Sources are planned; only the order of the apps they list differs.
func TestProwlarrAndArrsRadarrFirst(t *testing.T) {
	pr, so, ra := sameSetup()
	p := BuildPlan([]Input{{3, ra}, {2, so}, {1, pr}}, Choices{})
	if items := sourceItems(p); len(items) != 2 {
		t.Fatalf("%d Sources", len(items))
	}
	sa := item(t, p, GroupSources, "source:myjackett").Source.SameAs
	if len(sa) != 2 || sa[0].App != "radarr" || sa[1].App != "sonarr" {
		t.Fatalf("same as %+v", sa)
	}
	item(t, p, GroupSources, "source:nyaa.si (prowlarr)")
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
		if sp.SameAs[0].Name != other {
			t.Fatalf("same as %+v, want %q", sp.SameAs, other)
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
		{"together", [][]*Snapshot{{pr, so, ra}, {pr, so, ra}}, []string{"MyJackett", "Nyaa.si (Prowlarr)"}},
		{"arrs first", [][]*Snapshot{{so, ra}, {pr}, {pr, so, ra}}, []string{"MyJackett (Prowlarr)", "Nyaa.si (Prowlarr)"}},
		{"prowlarr first", [][]*Snapshot{{pr}, {so, ra}, {pr, so, ra}}, []string{"MyJackett", "Nyaa.si (Prowlarr)"}},
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
	if got := lineOf(t, mv.Report, "source:myjackett"); got.Result != ResAlready || got.Detail != "the same as MyJackett (Prowlarr), a Source that is here" {
		t.Fatalf("line %+v", got)
	}
}

// TestSameIndexerKeepsMultiLanguages: what MULTi means in each app comes
// over on the one Source, whichever of them was read first.
func TestSameIndexerKeepsMultiLanguages(t *testing.T) {
	pr, so, ra := sameSetup()
	so.Indexers[1].Settings["multiLanguages"] = []any{float64(2)}
	ra.Indexers[1].Settings["multiLanguages"] = []any{float64(2), float64(4)}
	for _, in := range [][]Input{{{1, pr}, {2, so}, {3, ra}}, {{2, so}, {3, ra}, {1, pr}}} {
		jk := item(t, BuildPlan(in, Choices{}), GroupSources, "source:myjackett").Source
		if !slices.Equal(jk.MultiLanguages, []string{"fr", "de"}) {
			t.Fatalf("MULTi languages %v", jk.MultiLanguages)
		}
	}
}

// TestRemovedSourceIsNotTheSame: a Source removed here does not keep the
// same indexer from coming over again.
func TestRemovedSourceIsNotTheSame(t *testing.T) {
	l := newMoveLab(t)
	r := alreadyRun(t, l)
	copied := Item{Key: "source:a", Name: "MyJackett (Prowlarr)", Source: &SourcePlan{Format: "torznab", Protocol: "torrent", URL: "http://prowlarr:9696/7/api", Priority: 25}}
	if got, why := r.one(GroupSources, copied); got != ResCreated {
		t.Fatalf("copy %s: %s", got, why)
	}
	own := Item{Key: "source:b", Name: "MyJackett", Source: &SourcePlan{Format: "torznab", Protocol: "torrent", URL: "http://jackett:9117/api/v2.0/indexers/x/results/torznab", Priority: 25, Through: []string{"http://prowlarr:9696/7/api"}}}
	if got, why := r.one(GroupSources, own); got != ResAlready || why != "the same as MyJackett (Prowlarr), a Source that is here" {
		t.Fatalf("own %s: %s", got, why)
	}
	if err := l.st.Write(r.ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE sources SET name = name || ' (removed ' || id || ')'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, why := r.one(GroupSources, own); got != ResCreated {
		t.Fatalf("after removing it %s: %s", got, why)
	}
}
