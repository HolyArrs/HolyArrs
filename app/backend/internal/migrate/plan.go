package migrate

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
)

// builder carries a plan being built.
type builder struct {
	p          *Plan
	rules      []PathRule
	skip       map[string]bool
	items      map[string]*Group
	keys       map[string]bool
	folders    []FolderPlan      // every folder planned (for title placement)
	presets    map[string]string // app + profile id -> preset
	titles     map[string]string // kind:id -> name (asks take names from here)
	planned    map[string]bool   // person keys planned
	personRefs map[string]string // app/source id -> stable person key
	choices    map[string]string // the owner's choices per person key
	// waits gathers the default wait rules per ready-made Pick rules name,
	// in the order first seen, so apps that disagree are seen once all are read.
	waits     map[string][]waitFrom
	waitOrder []string
	listNames map[string]bool // Outside list names planned (lower case)
	app       Kind
	snapID    int64
	paths     []pathRow // usable path rewrites from every app, placed by pathMaps
}

// waitFrom is one app's untagged default wait rule as Pick rules waits.
type waitFrom struct {
	app    Kind
	from   string
	plan   ProtocolPlan
	values string
	notes  string
}

// clip cleans a name from an old app: no control characters, trimmed,
// at most n runes.
func clip(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n]))
	}
	return s
}

// oldPath cleans a path as the old app saw it into slash form; "" when it
// is not an absolute path or climbs out of itself.
func oldPath(p string) string {
	p = clip(p, maxPath)
	if p == "" {
		return ""
	}
	if strings.Contains(p, `\`) || (len(p) >= 2 && p[1] == ':') {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if !strings.HasPrefix(p, "/") && !(len(p) >= 3 && p[1] == ':' && p[2] == '/') {
		return ""
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return ""
		}
	}
	c := path.Clean(p)
	if len(c) > 1 {
		c = strings.TrimSuffix(c, "/")
	}
	return c
}

// under says whether p is base or inside it.
func under(p, base string) bool {
	if p == base {
		return true
	}
	if base == "/" {
		return strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, base+"/")
}

// MapPath maps an old path through the rules (the longest matching From
// wins); without a match the path stays as it is. ok is false when the
// path is not usable (relative, or climbing out of itself).
func MapPath(rules []PathRule, p string) (string, bool) {
	op := oldPath(p)
	if op == "" {
		return "", false
	}
	best, bestLen := -1, -1
	for i, r := range rules {
		from := oldPath(r.From)
		if from == "" || oldPath(r.To) == "" {
			continue
		}
		if under(op, from) && len(from) > bestLen {
			best, bestLen = i, len(from)
		}
	}
	if best < 0 {
		return op, true
	}
	from, to := oldPath(rules[best].From), oldPath(rules[best].To)
	rest := strings.TrimPrefix(op, from)
	if from == "/" {
		rest = op
	}
	return path.Clean(to + "/" + strings.TrimPrefix(rest, "/")), true
}

// ValidRules checks the owner's path rules: both sides absolute paths
// that never climb out of themselves, at most 100 rules.
func ValidRules(rules []PathRule) error {
	if len(rules) > 100 {
		return &RuleError{"at most 100 path rules"}
	}
	for _, r := range rules {
		if oldPath(r.From) == "" || oldPath(r.To) == "" {
			return &RuleError{fmt.Sprintf("path rule %q to %q: both sides must be full paths without ..", clip(r.From, 100), clip(r.To, 100))}
		}
	}
	return nil
}

// RuleError is a path rule the move cannot use; its text is safe to show.
type RuleError struct{ Text string }

// Error returns the rule text.
func (e *RuleError) Error() string { return e.Text }

// BuildPlan turns Snapshots into a Plan.
func BuildPlan(in []Input, ch Choices) *Plan {
	p := &Plan{covered: map[string]bool{}, Snapshots: []int64{}, Apps: []string{}, NotMoved: []NotMoved{}, NeedsInput: []string{}, Paths: []string{}}
	b := &builder{p: p, rules: ch.PathRules, skip: map[string]bool{}, items: map[string]*Group{}, keys: map[string]bool{},
		presets: map[string]string{}, titles: map[string]string{}, planned: map[string]bool{}, personRefs: map[string]string{}, choices: ch.People,
		waits: map[string][]waitFrom{}, listNames: map[string]bool{}}
	for _, s := range ch.Skip {
		b.skip[s] = true
	}
	for _, name := range GroupOrder {
		p.Groups = append(p.Groups, Group{Name: name, Label: GroupLabels[name], Skipped: b.skip[name], Items: []Item{}})
	}
	for i := range p.Groups {
		b.items[p.Groups[i].Name] = &p.Groups[i]
	}
	// Order: rules and folders before titles; titles before asks; people before requests.
	order := func(k Kind) int {
		switch k {
		case Seerr, Overseerr:
			return 2
		case Prowlarr, Bazarr, QBittorrent, SABnzbd:
			return 1
		}
		return 0
	}
	sorted := append([]Input(nil), in...)
	sort.SliceStable(sorted, func(i, j int) bool { return order(sorted[i].Snap.Kind) < order(sorted[j].Snap.Kind) })
	for _, x := range sorted {
		if x.Snap == nil {
			continue
		}
		b.app, b.snapID = x.Snap.Kind, x.ID
		p.Snapshots = append(p.Snapshots, x.ID)
		p.Apps = append(p.Apps, string(x.Snap.Kind))
		b.snapshot(x.Snap)
	}
	b.waitItems()
	b.pathMaps()
	p.NeedsInput = noteList(p.NeedsInput)
	p.Notes = noteList(p.Notes)
	p.Paths = noteList(p.Paths)
	return p
}

func (b *builder) from(field string, i int) string {
	if i < 0 {
		return string(b.app) + " " + field
	}
	return fmt.Sprintf("%s %s[%d]", b.app, field, i)
}

func (b *builder) not(group, from, what, why string) {
	b.p.NotMoved = append(b.p.NotMoved, NotMoved{Group: group, From: from, What: clip(what, maxName), Why: why})
}

// add puts an item in its group once; a second item with the same key is
// listed as not moved (the first one wins).
func (b *builder) add(group string, it Item) {
	it.Name = clip(it.Name, maxName)
	if it.Name == "" {
		it.Name = "(no name)"
	}
	g := b.items[group]
	limit := MaxPlanOthers
	if group == GroupTitles {
		limit = MaxPlanTitles
	}
	switch {
	case b.keys[it.Key]:
		b.not(group, it.From, it.Name, "the same thing came from another app or twice from this one; it is moved once")
		return
	case len(g.Items) >= limit:
		b.not(group, it.From, it.Name, fmt.Sprintf("more than %d in one move; move again after this one", limit))
		return
	}
	b.keys[it.Key] = true
	g.Items = append(g.Items, it)
}

func (b *builder) cover(fields ...string) {
	for _, f := range fields {
		b.p.covered[f] = true
	}
}

func (b *builder) snapshot(s *Snapshot) {
	// The read's own facts are not settings.
	b.cover("kind", "via", "version", "schema", "read_at")
	// An import list's sign-in has nowhere to go here: its own Not moved
	// line says so (leftBehind) instead of asking for it.
	var needs []string
	for _, n := range s.NeedsReentry {
		if !strings.HasPrefix(n, `import list "`) {
			needs = append(needs, n)
		}
	}
	b.p.NeedsInput = append(b.p.NeedsInput, prefix(string(s.Kind), needs)...)
	b.cover("needs_reentry")
	b.p.Notes = append(b.p.Notes, prefix(string(s.Kind), s.Notes)...)
	b.cover("notes")

	b.folderList(s)
	b.profiles(s)
	b.titleList(s)
	b.exclusions(s)
	b.indexers(s)
	b.downloaders(s)
	b.pathMappings(s)
	b.naming(s)
	b.subtitles(s)
	b.people(s)
	b.asks(s)
	b.alerts(s)
	b.leftBehind(s)
}

func prefix(app string, in []string) []string {
	out := make([]string, 0, len(in))
	for _, x := range in {
		out = append(out, app+": "+clip(x, 300))
	}
	return out
}

// libraryKind is what kind of Library folder an app's folders are.
func libraryKind(k Kind) string {
	switch k {
	case Radarr:
		return "movie"
	case Sonarr:
		return "show"
	case Lidarr:
		return "music"
	}
	return ""
}
