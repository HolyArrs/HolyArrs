package migrate

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Prowlarr fills Sonarr and Radarr with its indexers: each one is a Torznab
// or Newznab indexer there at Prowlarr's per-site address
// (http://prowlarr:9696/5/api), named "<the name in Prowlarr> (Prowlarr)".
// Imported together, the same site would come over once from each app; it
// comes over once. The rules, kept in sourceIDs:
//
//   - an indexer in another app at a Prowlarr per-site address of an imported
//     Prowlarr (its address as read or as its app links give it to the apps,
//     plus /<id>/api) is that Prowlarr indexer;
//   - with another host name, it is still that Prowlarr indexer when the id
//     is the same and its name is Prowlarr's name plus " (Prowlarr)" (the
//     apps never send the key, so the key cannot be compared);
//   - two Torznab, Newznab or RSS indexers at the same address (letter case,
//     the default port and a trailing / aside) are one.
//
// The one read from Prowlarr itself wins, whichever app was read first: it
// is the feed itself, not Prowlarr's copy of it. The others go into it, with
// their media kinds and categories, and are listed on it (SameAs).

// srcSeen is a Source already planned and what makes another one the same.
type srcSeen struct {
	at     int // its place in the Sources group
	ids    map[string]bool
	native bool // read from Prowlarr itself
}

const prowlarrSuffix = " (Prowlarr)"

// prowlarrName is a name without the " (Prowlarr)" Prowlarr adds to the
// indexers it puts in the other apps, and whether it had it.
func prowlarrName(name string) (string, bool) {
	if len(name) > len(prowlarrSuffix) && strings.EqualFold(name[len(name)-len(prowlarrSuffix):], prowlarrSuffix) {
		return strings.TrimSpace(name[:len(name)-len(prowlarrSuffix)]), true
	}
	return name, false
}

// addrKey is an address to compare: scheme://host:port/path in lower case,
// the default port filled in, no trailing /, the query kept; "" when it is
// not an http or https address.
func addrKey(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	k := originOf(raw)
	if p := strings.Trim(u.Path, "/"); p != "" {
		k += "/" + strings.ToLower(p)
	}
	if u.RawQuery != "" {
		k += "?" + u.RawQuery
	}
	return k
}

// prowlarrFeed splits a Prowlarr per-site address
// (http://prowlarr:9696/5/api) into Prowlarr's address and the indexer id.
func prowlarrFeed(addr string) (string, int64, bool) {
	u, err := url.Parse(addr)
	if err != nil {
		return "", 0, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || !strings.EqualFold(parts[len(parts)-1], "api") {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	u.Path = "/" + strings.Join(parts[:len(parts)-2], "/")
	u.RawQuery, u.Fragment = "", ""
	base := addrKey(u.String())
	return base, id, base != ""
}

// prowlarrBases are the addresses the other apps reach this Prowlarr at:
// the one it was read at and the one each of its app links gives.
func prowlarrBases(s *Snapshot) []string {
	var out []string
	for _, raw := range append([]string{s.Address}, linkAddrs(s)...) {
		if k := addrKey(raw); k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

func linkAddrs(s *Snapshot) []string {
	var out []string
	for _, l := range s.AppLinks {
		out = append(out, setStr(l.Settings, "prowlarrUrl"))
	}
	return out
}

// sourceIDs are what make another indexer the same Source as this one.
func sourceIDs(s *Snapshot, ix Provider, sp *SourcePlan) map[string]bool {
	ids := map[string]bool{}
	if k := addrKey(sp.URL); k != "" {
		ids["feed:"+sp.Format+":"+k] = true
	}
	if s.Kind == Prowlarr {
		if ix.ID > 0 {
			for _, base := range prowlarrBases(s) {
				ids["prowlarr:"+base+"#"+strconv.FormatInt(ix.ID, 10)] = true
			}
			ids["prowlarr-name:"+strconv.FormatInt(ix.ID, 10)+":"+strings.ToLower(clip(ix.Name, 80))] = true
		}
		return ids
	}
	if sp.Format == "torznab" || sp.Format == "newznab" {
		if base, id, ok := prowlarrFeed(sp.URL); ok {
			ids["prowlarr:"+base+"#"+strconv.FormatInt(id, 10)] = true
			if name, ok := prowlarrName(clip(ix.Name, 80)); ok {
				ids["prowlarr-name:"+strconv.FormatInt(id, 10)+":"+strings.ToLower(name)] = true
			}
		}
	}
	return ids
}

// through are the per-site addresses the other apps reach a Prowlarr
// indexer at.
func through(s *Snapshot, ix Provider) []string {
	if s.Kind != Prowlarr || ix.ID <= 0 {
		return nil
	}
	var out []string
	for _, base := range prowlarrBases(s) {
		out = append(out, strings.Join([]string{base, strconv.FormatInt(ix.ID, 10), "api"}, "/"))
	}
	return out
}

// addSource plans a Source, or puts it into the same one planned already.
func (b *builder) addSource(s *Snapshot, ix Provider, it Item, tagged bool) {
	ids := sourceIDs(s, ix, it.Source)
	native := s.Kind == Prowlarr
	it.Source.Through = through(s, ix)
	g := b.items[GroupSources]
	for i := range b.srcs {
		m := &b.srcs[i]
		if !overlap(m.ids, ids) {
			continue
		}
		have := &g.Items[m.at]
		if native && !m.native && (have.Key == it.Key || !b.keys[it.Key]) {
			// Prowlarr's own indexer takes the place of its copies in the
			// other apps, which go into it.
			was := *have
			it.LegacyKeys = appendNew(appendNew(it.LegacyKeys, was.Key), was.LegacyKeys...)
			it.Source.SameAs = append([]SameIndexer{{App: appOf(was.From), Name: was.Name, URL: was.Source.URL}}, was.Source.SameAs...)
			foldSource(it.Source, was.Source)
			b.keys[it.Key] = true
			*have = it
			m.native = true
			b.p.Mapped = append(b.p.Mapped, was.From)
			b.dropNeeds(appOf(was.From), was.Name)
			for _, x := range was.Source.SameAs {
				b.dropNeeds(x.App, x.Name)
			}
		} else {
			foldSource(have.Source, it.Source)
			have.Source.SameAs = append(have.Source.SameAs, SameIndexer{App: string(s.Kind), Name: it.Name, URL: it.Source.URL})
			if it.Key != have.Key {
				have.LegacyKeys = appendNew(have.LegacyKeys, it.Key)
			}
			have.Source.Through = appendNew(have.Source.Through, it.Source.Through...)
			b.p.Mapped = append(b.p.Mapped, it.From)
			b.dropNeeds(string(s.Kind), it.Name)
		}
		for id := range ids {
			m.ids[id] = true
		}
		return
	}
	before := len(g.Items)
	if tagged {
		b.addTagged(s.Kind, GroupSources, it, ix.Tags)
	} else {
		b.add(GroupSources, it)
	}
	if len(g.Items) > before {
		b.srcs = append(b.srcs, srcSeen{at: before, ids: ids, native: native})
	}
}

func overlap(a, b map[string]bool) bool {
	for k := range b {
		if a[k] {
			return true
		}
	}
	return false
}

func appendNew(to []string, xs ...string) []string {
	for _, x := range xs {
		if x != "" && !slices.Contains(to, x) {
			to = append(to, x)
		}
	}
	return to
}

// appOf is the app a plan line came from ("sonarr indexers[2]": sonarr).
func appOf(from string) string {
	app, _, _ := strings.Cut(from, " ")
	return app
}

// foldSource gives a Source the media kinds, categories and MULTi languages
// of the same one in another app.
func foldSource(to, from *SourcePlan) {
	to.On = to.On || from.On
	if to.MinSeeders == nil && from.MinSeeders != nil && to.Protocol == "torrent" {
		v := *from.MinSeeders
		to.MinSeeders = &v
	}
	for _, k := range from.Kinds {
		if !slices.Contains(to.Kinds, k) {
			to.Kinds = append(to.Kinds, k)
		}
	}
	for _, k := range to.Kinds {
		if cs := from.Categories[k]; len(cs) > 0 && len(to.Categories[k]) == 0 {
			if to.Categories == nil {
				to.Categories = map[string][]int{}
			}
			to.Categories[k] = slices.Clone(cs)
		}
	}
	to.MultiLanguages = appendNew(to.MultiLanguages, from.MultiLanguages...)
}

// dropNeeds takes an indexer that went into another Source off the list of
// keys to type: the Source it went into asks for its own.
func (b *builder) dropNeeds(app, name string) {
	lead := app + ": indexer \"" + name + "\": "
	b.p.NeedsInput = slices.DeleteFunc(b.p.NeedsInput, func(n string) bool { return strings.HasPrefix(n, lead) })
}
