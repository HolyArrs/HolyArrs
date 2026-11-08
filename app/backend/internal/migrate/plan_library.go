package migrate

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
)

func (b *builder) folderList(s *Snapshot) {
	b.cover("folders")
	kind := libraryKind(s.Kind)
	for i, f := range s.Folders {
		from := b.from("folders", i)
		op := oldPath(f.Path)
		if kind == "" || op == "" {
			b.not(GroupFolders, from, f.Path, "not a full path to a library folder")
			continue
		}
		np, _ := MapPath(b.rules, op)
		b.p.Paths = append(b.p.Paths, op)
		label := path.Base(np)
		b.folders = append(b.folders, FolderPlan{OldPath: op, Path: np, Kind: kind, Label: label})
		detail := "at " + np
		if np != op {
			detail = op + " becomes " + np
		}
		b.add(GroupFolders, Item{Key: "folder:" + np, Name: label, From: from, Detail: detail,
			Folder: &FolderPlan{OldPath: op, Path: np, Kind: kind, Label: clip(label, 100)}})
		keys := make([]string, 0, len(f.Extra))
		for k := range f.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.not(GroupFolders, from+"."+k, k, "a library folder here has a path, a kind and default Pick rules only")
		}
	}
}

// matchPreset finds the closest Pick rules preset for one of their
// profiles and says why.
func matchPreset(k Kind, p Profile) (string, string) {
	all := strings.ToLower(p.Cutoff + " " + strings.Join(p.Allowed, " "))
	scores := ""
	if len(p.Scores) > 0 {
		scores = fmt.Sprintf("; its %d custom format scores are not moved (Pick rules score releases their own way)", len(p.Scores))
	}
	if k == Lidarr {
		lossless := strings.Contains(all, "flac") || strings.Contains(all, "alac") || strings.Contains(all, "wav") || strings.Contains(all, "lossless")
		lossy := strings.Contains(all, "mp3") || strings.Contains(all, "aac") || strings.Contains(all, "ogg") || strings.Contains(all, "v0") || strings.Contains(all, "320")
		switch {
		case lossless && strings.Contains(strings.ToLower(p.Cutoff), "flac") || lossless && !lossy:
			return "Lossless first", "it wants lossless audio first (" + clip(p.Cutoff, 60) + ")" + scores
		case lossy && (strings.Contains(all, "320") || strings.Contains(all, "v0")) && !lossless:
			return "Lossy 320 or V0", "it allows only good lossy audio (" + clip(p.Cutoff, 60) + ")" + scores
		}
		return "Any music", "it allows a mix of audio kinds" + scores
	}
	cut := strings.ToLower(p.Cutoff)
	if strings.Contains(cut, "2160") || strings.Contains(cut, "4k") || strings.Contains(cut, "uhd") {
		return "4K", "it aims for 2160p (" + clip(p.Cutoff, 60) + ")" + scores
	}
	why := "it aims for " + clip(p.Cutoff, 60) + " or lower; Standard picks the best copy up to 1080p"
	if p.Cutoff == "" {
		why = "it names no top quality; Standard picks the best copy up to 1080p"
	}
	if !p.UpgradeAllowed {
		why += "; it never looked for better copies, Standard does until the top step"
	}
	return "Standard", why + scores
}

func (b *builder) profiles(s *Snapshot) {
	b.cover("quality_profiles", "custom_formats", "release_profiles", "delay_profiles", "language_profiles", "metadata_profiles")
	for i, p := range s.QualityProfiles {
		preset, why := matchPreset(s.Kind, p)
		b.presets[string(s.Kind)+":"+strconv.FormatInt(p.ID, 10)] = preset
		b.add(GroupRules, Item{Key: fmt.Sprintf("pick_rules:%d:%s:%d", b.snapID, s.Kind, p.ID), Name: p.Name, From: b.from("quality_profiles", i),
			Detail: "becomes " + preset, Why: why, Rules: &RulesPlan{App: string(s.Kind), Profile: clip(p.Name, maxName), Preset: preset}})
	}
	for i, f := range s.CustomFormats {
		b.not(GroupRules, b.from("custom_formats", i), f.Name, "format scoring rules do not move; Pick rules score a fixed list of traits")
	}
	for i, rw := range s.ReleaseProfiles {
		name := rw.Name
		if name == "" {
			name = "release words"
		}
		var parts []string
		if w := wordList(rw.Settings["required"]); w != "" {
			parts = append(parts, "must contain "+w)
		}
		if w := wordList(rw.Settings["ignored"]); w != "" {
			parts = append(parts, "must not contain "+w)
		}
		// The words are cut short enough that the scope always fits maxName.
		what := name
		if len(parts) > 0 {
			what += ": " + strings.Join(parts, " / ")
		}
		scope := tagScope(wordList(rw.Settings["tags"]))
		if w := wordList(rw.Settings["excluded_tags"]); w != "" {
			scope = strings.TrimSuffix(scope, ")") + " except tags: " + clip(w, 40) + ")"
		}
		what = clip(what, 90) + " " + scope
		why := "Pick rules test a fixed list of traits, not free words; add a must-have or never trait under Pick rules if one fits"
		if !boolean(rw.Settings["enabled"]) {
			why = "it was turned off there"
		}
		b.not(GroupRules, b.from("release_profiles", i), what, why)
	}
	b.waitRules(s)
	for i, n := range s.LanguageProfiles {
		b.not(GroupRules, b.from("language_profiles", i), n.Name, "language profiles are not moved; Pick rules set the languages")
	}
	// Lidarr metadata profiles are accounted where a moved artist actually
	// consumes their individual fields in titleList.
}

// maxWait is the longest wait Pick rules allow, in minutes (7 days).
const maxWait = 10080

// waitValues says a wait rule's values ("prefer torrent, Usenet 0 min,
// torrent 120 min, Usenet off").
func waitValues(d DelayProfile) string {
	prefer := d.PreferredProtocol
	if prefer == "" {
		prefer = "either"
	}
	values := fmt.Sprintf("prefer %s, Usenet %d min, torrent %d min", prefer, d.UsenetDelay, d.TorrentDelay)
	if !d.EnableUsenet {
		values += ", Usenet off"
	}
	if !d.EnableTorrent {
		values += ", torrent off"
	}
	return values
}

// waitWords says Pick rules waits in the plan and the report.
func waitWords(p ProtocolPlan) string {
	s := fmt.Sprintf("prefer %s, Usenet %d min, torrent %d min", p.Prefer, p.WaitUsenet, p.WaitTorrent)
	if p.SkipAtGoodEnough {
		s += ", no wait once a release is Good enough"
	}
	return s
}

func clampWait(n int) (int, bool) {
	switch {
	case n > maxWait:
		return maxWait, true
	case n < 0:
		return 0, false
	}
	return n, false
}

// waitRules handles an app's wait rules. Its untagged default rule (the
// untagged one with the highest order) is gathered for the ready-made Pick
// rules its quality profiles matched (waitItems), unless it is the old app's
// untouched default, which would only switch them to prefer Usenet. Tagged
// rules are listed with their values.
func (b *builder) waitRules(s *Snapshot) {
	def := -1
	for i, d := range s.DelayProfiles {
		if len(d.Tags) == 0 && (def < 0 || d.Order > s.DelayProfiles[def].Order) {
			def = i
		}
	}
	var presets []string
	for _, p := range s.QualityProfiles {
		if pr := b.presets[string(s.Kind)+":"+strconv.FormatInt(p.ID, 10)]; pr != "" && !slices.Contains(presets, pr) {
			presets = append(presets, pr)
		}
	}
	for i, d := range s.DelayProfiles {
		from := b.from("delay_profiles", i)
		values := waitValues(d)
		what := "wait before grabbing " + tagScope(strings.Join(d.Tags, ", "))
		switch {
		case len(d.Tags) > 0:
			b.not(GroupRules, from, what, "applied only to titles tagged "+clip(strings.Join(d.Tags, ", "), 100)+"; HolyArrs has no tags, so its waits ("+values+") were not moved; set them on a copy of the Pick rules if those titles need them")
			continue
		case i != def || mediaWord(s.Kind) == "":
			b.not(GroupRules, from, what, "HolyArrs has waits in Pick rules (prefer, Usenet and torrent minutes) but this one was not carried over: "+values+"; set them under Pick rules")
			continue
		}
		waits := d.UsenetDelay > 0 || d.TorrentDelay > 0
		if d.BypassIfAboveScore && waits {
			b.not(GroupRules, from+".bypass_score", "skip the wait at a format score", "format scores do not move; turn on \"Skip the wait once good enough\" in Pick rules instead")
		}
		if d.EnableUsenet && d.EnableTorrent && strings.EqualFold(d.PreferredProtocol, "usenet") && !waits {
			// The old app's untouched default (its skip switches do nothing
			// without a wait): it grabs at once, which is what the ready-made
			// Pick rules do, so it only takes part in the disagreement check.
			if len(presets) == 0 {
				b.p.Mapped = append(b.p.Mapped, from)
			}
			for _, pr := range presets {
				if _, ok := b.waits[pr]; !ok {
					b.waitOrder = append(b.waitOrder, pr)
				}
				b.waits[pr] = append(b.waits[pr], waitFrom{app: s.Kind, from: from, plan: ProtocolPlan{Preset: pr, Prefer: "either"}, values: values})
			}
			continue
		}
		w := ProtocolPlan{Prefer: "either", SkipAtGoodEnough: d.BypassIfHighestQuality && waits}
		if p := strings.ToLower(d.PreferredProtocol); p == "usenet" || p == "torrent" {
			w.Prefer = p
		}
		notes := ""
		var cut bool
		if w.WaitUsenet, cut = clampWait(d.UsenetDelay); cut {
			notes += fmt.Sprintf("; Usenet %d min cut to %d (HolyArrs allows up to 7 days)", d.UsenetDelay, maxWait)
		}
		if w.WaitTorrent, cut = clampWait(d.TorrentDelay); cut {
			notes += fmt.Sprintf("; torrent %d min cut to %d (HolyArrs allows up to 7 days)", d.TorrentDelay, maxWait)
		}
		if w.SkipAtGoodEnough {
			notes += "; HolyArrs skips the wait at Good enough, which can be below the top allowed quality"
		}
		switch {
		case d.EnableUsenet && !d.EnableTorrent:
			w.Prefer = "usenet"
			b.not(GroupRules, from+".enable_torrent", "torrent off", "Pick rules cannot turn a protocol off, so pause the torrent Sources instead; Usenet is preferred")
		case d.EnableTorrent && !d.EnableUsenet:
			w.Prefer = "torrent"
			b.not(GroupRules, from+".enable_usenet", "Usenet off", "Pick rules cannot turn a protocol off, so pause the Usenet Sources instead; torrent is preferred")
		case !d.EnableTorrent && !d.EnableUsenet:
			b.not(GroupRules, from+".enable_usenet", "Usenet and torrent off", "Pick rules cannot turn a protocol off, so pause those Sources instead")
		}
		if len(presets) == 0 {
			b.not(GroupRules, from, what, "no quality profile came over to carry it: "+values+"; set them under Pick rules")
			continue
		}
		for _, pr := range presets {
			wp := w
			wp.Preset = pr
			if _, ok := b.waits[pr]; !ok {
				b.waitOrder = append(b.waitOrder, pr)
			}
			b.waits[pr] = append(b.waits[pr], waitFrom{app: s.Kind, from: from, plan: wp, values: values, notes: notes})
		}
	}
}

// waitItems plans, once every app is read, one wait change per ready-made
// Pick rules. Apps that set different waits for the same Pick rules change
// nothing there, and each one is listed.
func (b *builder) waitItems() {
	for _, preset := range b.waitOrder {
		ws := b.waits[preset]
		same := true
		for _, w := range ws[1:] {
			same = same && w.plan == ws[0].plan
		}
		if !same {
			for _, w := range ws {
				b.not(GroupRules, w.from, "wait before grabbing (every title)",
					fmt.Sprintf("the old apps set different waits for %s, so this server's wait stays; %s had %s", preset, w.app, w.values))
			}
			continue
		}
		p := ws[0].plan
		if p == (ProtocolPlan{Preset: preset, Prefer: "either"}) {
			for _, w := range ws {
				if !slices.Contains(b.p.Mapped, w.from) {
					b.p.Mapped = append(b.p.Mapped, w.from)
				}
			}
			continue
		}
		var apps []string
		for i, w := range ws {
			if !slices.Contains(apps, string(w.app)) {
				apps = append(apps, string(w.app))
			}
			// The part the change belongs to, when every app that set it
			// has the same one; "" (always moved) when they differ.
			part := map[Kind]string{Radarr: "movies", Sonarr: "shows", Lidarr: "music"}[w.app]
			if i == 0 {
				p.Part = part
			} else if p.Part != part {
				p.Part = ""
			}
		}
		b.add(GroupRules, Item{Key: "pick_rules_protocol:" + preset, Name: preset + " wait", From: ws[0].from,
			Detail:   "from the default wait rule of " + strings.Join(apps, " and ") + ": " + waitWords(p) + ws[0].notes,
			Why:      "set only while the wait on these ready-made Pick rules is still as it came; undo puts it back",
			Protocol: &p})
		for _, w := range ws[1:] {
			if !slices.Contains(b.p.Mapped, w.from) {
				b.p.Mapped = append(b.p.Mapped, w.from)
			}
		}
	}
}

// tagScope says which titles a rule served: "(tags: a, b)" or "(every title)".
func tagScope(tags string) string {
	if tags == "" {
		return "(every title)"
	}
	return "(tags: " + clip(tags, 100) + ")"
}

// wordList joins a kept list of words or names ("" when there are none).
func wordList(v any) string {
	var out []string
	for _, e := range list(v) {
		if w := str(e); w != "" {
			out = append(out, w)
		}
	}
	return clip(strings.Join(out, ", "), 120)
}

// mediaWord is the kind of title an app's links served ("" for the others).
func mediaWord(k Kind) string {
	return map[Kind]string{Radarr: "movie", Sonarr: "show", Lidarr: "music"}[k]
}

// tagNote is the Detail words for a moved Source, Downloader or Alert of
// Radarr, Sonarr or Lidarr that served only tagged titles there ("" when
// it had no tags).
func tagNote(k Kind, tags []string) string {
	if mediaWord(k) == "" || len(tags) == 0 {
		return ""
	}
	return "; only for titles tagged " + clip(strings.Join(tags, ", "), 100) + " there; here it serves every " + mediaWord(k) + " title"
}

// addTagged adds a Source, Downloader or Alert and, once it is planned and
// had tags, lists its tags as not moved.
func (b *builder) addTagged(k Kind, group string, it Item, tags []string) {
	note := tagNote(k, tags)
	it.Detail += note
	before := len(b.items[group].Items)
	b.add(group, it)
	if note != "" && len(b.items[group].Items) > before {
		check := "HolyArrs has no tags; check it before you resume it"
		if group == GroupAlerts {
			check = "HolyArrs has no tags; check it before you turn it on"
		}
		b.not(group, it.From+".tags", clip(it.Name, 80)+" (tags: "+clip(strings.Join(tags, ", "), 100)+")", check)
	}
}

// sharesTag says whether two tag lists have a name in common.
func sharesTag(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}

func allowedMusicValues(v any, names map[string]string) (out, unsupported []string) {
	for _, raw := range list(v) {
		m := lower(obj(raw))
		if !boolean(m["allowed"]) {
			continue
		}
		name := str(m["name"])
		if n := names[strings.ToLower(name)]; n != "" {
			out = append(out, n)
		} else {
			unsupported = append(unsupported, name)
		}
	}
	return out, unsupported
}

type musicIssue struct{ field, what, why string }

func musicPolicy(s *Snapshot, t Title) (*MusicPolicyPlan, []musicIssue) {
	if s.Kind != Lidarr {
		return nil, nil
	}
	p := &MusicPolicyPlan{Albums: []MusicAlbumPlan{}}
	var issues []musicIssue
	switch strings.ToLower(str(anyKey(t.Extra, "new_albums"))) {
	case "new":
		v := "future_only"
		p.NewAlbums = &v
	case "none":
		v := "none"
		p.NewAlbums = &v
	case "all":
		v := "all"
		p.NewAlbums = &v
	case "":
		issues = append(issues, musicIssue{"new_albums", "(missing)", "the source did not provide a new-album policy, so it is left unchanged"})
	default:
		issues = append(issues, musicIssue{"new_albums", str(anyKey(t.Extra, "new_albums")), "the new-album policy is unknown, so it is left unchanged"})
	}
	profileID := num(anyKey(t.Extra, "metadata_profile_id"))
	found := false
	if profileID == 0 {
		issues = append(issues, musicIssue{"metadata_profile_id", "(missing)", "the source did not identify a metadata profile, so its album filters are left unchanged"})
	}
	for _, mp := range s.MetadataProfiles {
		if mp.ID != profileID {
			continue
		}
		found = true
		var bad []string
		p.AlbumTypes, bad = allowedMusicValues(anyKey(mp.Settings, "primaryalbumtypes"), map[string]string{
			"album": "album", "ep": "ep", "single": "single", "broadcast": "broadcast", "other": "other",
		})
		for _, v := range bad {
			issues = append(issues, musicIssue{"metadata_profile.primaryalbumtypes", v, "this allowed album kind has no exact HolyArrs match"})
		}
		if len(p.AlbumTypes) == 0 {
			issues = append(issues, musicIssue{"metadata_profile.primaryalbumtypes", "(none supported)", "no supported allowed album kind was provided, so this filter is left unchanged"})
		}
		p.SecondaryTypes, bad = allowedMusicValues(anyKey(mp.Settings, "secondaryalbumtypes"), map[string]string{
			"studio": "studio", "live": "live", "compilation": "compilation", "remix": "remix", "soundtrack": "soundtrack", "spokenword": "spokenword", "audiobook": "audiobook", "interview": "interview", "demo": "demo",
		})
		for _, v := range bad {
			issues = append(issues, musicIssue{"metadata_profile.secondaryalbumtypes", v, "this allowed extra album kind has no exact HolyArrs match"})
		}
		if len(p.SecondaryTypes) == 0 {
			issues = append(issues, musicIssue{"metadata_profile.secondaryalbumtypes", "(none supported)", "no supported allowed extra album kind was provided, so this filter is left unchanged"})
		}
		p.ReleaseStatuses, bad = allowedMusicValues(anyKey(mp.Settings, "releasestatuses"), map[string]string{
			"official": "official", "promotion": "promotion", "bootleg": "bootleg", "pseudo-release": "pseudo-release", "pseudorelease": "pseudo-release",
		})
		for _, v := range bad {
			issues = append(issues, musicIssue{"metadata_profile.releasestatuses", v, "this allowed release status has no exact HolyArrs match"})
		}
		if len(p.ReleaseStatuses) == 0 {
			issues = append(issues, musicIssue{"metadata_profile.releasestatuses", "(none supported)", "no supported allowed release status was provided, so this filter is left unchanged"})
		}
		break
	}
	if profileID != 0 && !found {
		issues = append(issues, musicIssue{"metadata_profile_id", strconv.FormatInt(profileID, 10), "the referenced metadata profile was not read, so its filters are left unchanged"})
	}
	for i, a := range t.MusicAlbums {
		if !isMBID(a.MBID) {
			issues = append(issues, musicIssue{fmt.Sprintf("music_albums[%d].mbid", i), a.MBID, "the album id is invalid, so its monitoring choice is left unchanged"})
			continue
		}
		ap := MusicAlbumPlan{MBID: strings.ToLower(a.MBID), KeepUpToDate: a.Monitored}
		if isMBID(a.WatchedEdition) {
			ap.PressingMBID = strings.ToLower(a.WatchedEdition)
		} else if a.WatchedEdition != "" {
			issues = append(issues, musicIssue{fmt.Sprintf("music_albums[%d].watched_edition", i), a.WatchedEdition, "the pressing id is invalid, so the watched pressing is left unchanged"})
		}
		p.Albums = append(p.Albums, ap)
	}
	sort.Slice(p.Albums, func(i, j int) bool { return p.Albums[i].MBID < p.Albums[j].MBID })
	return p, issues
}

// idFor picks the id a title is added by.
func idFor(k Kind, t Title) (kind, id string) {
	switch k {
	case Radarr:
		if v := clip(t.IDs["tmdb"], 20); v != "" && isDigits(v) {
			return "movie", "tmdb:" + v
		}
		return "movie", ""
	case Sonarr:
		for _, src := range []string{"tvdb", "tmdb"} {
			if v := clip(t.IDs[src], 20); v != "" && isDigits(v) {
				return "show", src + ":" + v
			}
		}
		return "show", ""
	case Lidarr:
		if v := clip(t.IDs["musicbrainz"], 40); v != "" && isMBID(v) {
			return "artist", "mbid:" + strings.ToLower(v)
		}
		return "artist", ""
	}
	return "", ""
}

func isDigits(s string) bool {
	if s == "" || s == "0" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isMBID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range strings.ToLower(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// place finds the Library folder (here) and the one folder in it that hold
// a title's old path.
func (b *builder) place(t Title) (lib, folder, why string) {
	op := oldPath(t.Path)
	if op == "" {
		return "", "", "its folder is not a full path"
	}
	best := -1
	for i, f := range b.folders {
		if under(op, f.OldPath) && op != f.OldPath && (best < 0 || len(f.OldPath) > len(b.folders[best].OldPath)) {
			best = i
		}
	}
	if best < 0 {
		return "", "", "its folder " + op + " is in none of the library folders read"
	}
	f := b.folders[best]
	rest := strings.TrimPrefix(strings.TrimPrefix(op, f.OldPath), "/")
	if f.OldPath == "/" {
		rest = strings.TrimPrefix(op, "/")
	}
	if rest == "" || strings.Contains(rest, "/") || strings.HasPrefix(rest, ".") {
		return "", "", "its folder is not directly in its library folder"
	}
	return f.Path, rest, ""
}

// releaseGates are Radarr's release stages (words or the backup's numbers)
// as Release gate choices.
var releaseGates = map[string]string{"tba": "announced", "0": "announced", "announced": "announced", "1": "announced",
	"incinemas": "in_cinemas", "2": "in_cinemas", "released": "released", "3": "released"}

// gateWords say a Release gate choice in the plan's details.
var gateWords = map[string]string{"announced": "announced", "in_cinemas": "in cinemas", "released": "released", "": "this server's default"}

func (b *builder) titleList(s *Snapshot) {
	b.cover("titles")
	extras := map[string]int{}
	tags := 0
	usedMetadata := map[int64]bool{}
	// Radarr's availability delay is stamped on each moved movie (0 to 365
	// days); this server's default for new movies is left as it is.
	var delay *int
	delayNote := ""
	if v, ok := s.Settings["availability_delay"]; ok && s.Kind == Radarr {
		d := int(num(v))
		switch {
		case d > 365:
			d, delayNote = 365, ", cut to 365"
		case d < 0:
			d = 0
		}
		delay = &d
	}
	gated := 0
	for i, t := range s.Titles {
		from := b.from("titles", i)
		kind, id := idFor(s.Kind, t)
		if kind == "" {
			b.not(GroupTitles, from, t.Name, "this app has no library")
			continue
		}
		name := clip(t.Name, maxName)
		if id == "" {
			b.not(GroupTitles, from, name, "no "+map[string]string{"movie": "TMDB", "show": "TVDB or TMDB", "artist": "MusicBrainz"}[kind]+" id is known for it")
			continue
		}
		ids := map[string]string{}
		for k, v := range t.IDs {
			if k = clip(k, 20); k != "" && len(ids) < 10 {
				ids[k] = clip(v, 60)
			}
		}
		music, issues := musicPolicy(s, t)
		for _, issue := range issues {
			b.not(GroupTitles, from+"."+issue.field, issue.what, issue.why)
		}
		tp := &TitlePlan{Kind: kind, ID: id, IDs: ids, Year: t.Year, KeepUpToDate: t.Monitored,
			Preset: b.presets[string(s.Kind)+":"+strconv.FormatInt(t.ProfileID, 10)], Music: music}
		if tp.Preset == "" && kind != "artist" {
			tp.Preset = "Standard"
		}
		for _, se := range t.Seasons {
			if !se.Monitored && se.Number >= 0 && se.Number < 10000 {
				tp.SeasonsOff = append(tp.SeasonsOff, se.Number)
			}
		}
		gateNote := ""
		if kind == "movie" {
			if v := str(anyKey(t.Extra, "minimum_availability")); v != "" {
				if g, ok := releaseGates[strings.ToLower(v)]; ok {
					tp.ReleaseGate = g
				} else {
					b.not(GroupTitles, from+".minimum_availability", clip(name, 140)+": release stage "+clip(v, 40), "unknown release stage "+clip(v, 40)+"; the title takes this server's default")
				}
			}
			tp.GateDelayDays = delay
			if tp.ReleaseGate != "" || delay != nil {
				gateNote = "; release gate: " + gateWords[tp.ReleaseGate]
				if delay != nil {
					gateNote += fmt.Sprintf(" (+%d days%s)", *delay, delayNote)
				}
			}
		}
		tp.Library, tp.Folder, tp.Unresolved = b.place(t)
		b.titles[kind+":"+id] = name
		for _, v := range t.IDs {
			if v != "" {
				b.titles[kind+":ids:"+clip(v, 60)] = name
			}
		}
		if tp.Unresolved != "" {
			b.not(GroupTitles, from, name, "not placed: "+tp.Unresolved+"; add a path rule and plan again")
			continue
		}
		detail := "uses the folder " + path.Join(tp.Library, tp.Folder) + " where it is"
		if !t.Monitored {
			detail += "; Keep up to date off"
		}
		detail += gateNote
		if t.Year > 0 {
			name = fmt.Sprintf("%s (%d)", name, t.Year)
		}
		key := "title:" + kind + ":" + id
		before := len(b.items[GroupTitles].Items)
		b.add(GroupTitles, Item{Key: key, Name: name, From: from, Detail: detail, Title: tp})
		if tp.GateDelayDays != nil && len(b.items[GroupTitles].Items) > before {
			gated++
		}
		if s.Kind == Lidarr && len(b.items[GroupTitles].Items) > before {
			profileID := num(anyKey(t.Extra, "metadata_profile_id"))
			for j, mp := range s.MetadataProfiles {
				if mp.ID != profileID {
					continue
				}
				usedMetadata[profileID] = true
				fields := []struct {
					name   string
					values []string
				}{{"primaryalbumtypes", tp.Music.AlbumTypes}, {"secondaryalbumtypes", tp.Music.SecondaryTypes}, {"releasestatuses", tp.Music.ReleaseStatuses}}
				known := map[string]bool{}
				for _, field := range fields {
					known[field.name] = true
					fromProfile := b.from("metadata_profiles", j) + "." + field.name
					if len(field.values) > 0 {
						b.p.Mapped = append(b.p.Mapped, fromProfile)
					} else {
						b.not(GroupRules, fromProfile, mp.Name+" "+field.name, "no supported allowed value was represented on the moved artist")
					}
				}
				settingKeys := make([]string, 0, len(mp.Settings))
				for field := range mp.Settings {
					settingKeys = append(settingKeys, field)
				}
				sort.Strings(settingKeys)
				for _, field := range settingKeys {
					if !known[field] {
						b.not(GroupRules, b.from("metadata_profiles", j)+"."+field, mp.Name+" "+field, "this metadata profile field has no HolyArrs match")
					}
				}
				break
			}
		}
		for k := range t.Extra {
			if s.Kind == Lidarr && (k == "new_albums" || k == "metadata_profile_id") || s.Kind == Radarr && k == "minimum_availability" {
				continue
			}
			extras[k]++
		}
		tags += len(t.Tags)
	}
	keys := make([]string, 0, len(extras))
	for k := range extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.not(GroupTitles, b.from("titles", -1)+"."+k, fmt.Sprintf("%s (on %d titles)", k, extras[k]), "no HolyArrs setting matches it; titles take HolyArrs's own defaults")
	}
	if tags > 0 {
		b.not(GroupTitles, b.from("titles", -1)+".tags", fmt.Sprintf("tags on titles (%d)", tags), "HolyArrs has no tags")
	}
	if delay != nil {
		from := b.from("settings", -1) + ".availability_delay"
		if gated > 0 {
			b.p.Mapped = append(b.p.Mapped, from)
			b.p.Notes = append(b.p.Notes, fmt.Sprintf("%s: the availability delay (%d days) is set on each moved movie; this server's default for new movies stays as it is", s.Kind, *delay))
		} else {
			b.not(GroupTitles, from, "availability delay", "no moved movie takes it; set the delay under Settings > Movies and TV defaults if you want it")
		}
	}
	if s.Kind == Lidarr {
		for i, mp := range s.MetadataProfiles {
			if !usedMetadata[mp.ID] {
				b.not(GroupRules, b.from("metadata_profiles", i), mp.Name, "no moved artist uses this metadata profile")
			}
		}
	}
}

func (b *builder) exclusions(s *Snapshot) {
	b.cover("exclusions")
	kind := map[Kind]string{Radarr: "movie", Sonarr: "show", Lidarr: "artist"}[s.Kind]
	// A live read whose exclusions route failed says so in the reader's
	// notes (a read kept before live reads had the backup-only note).
	if slices.Contains(s.Notes, "the app has no exclusions list; skipped") || slices.Contains(s.Notes, "exclusion lists are read from a backup only") {
		b.not(GroupNeverAdd, b.from("exclusions", -1), "exclusions", "they could not be read through the API; read a backup to bring them")
	}
	for i, e := range s.Exclusions {
		from := b.from("exclusions", i)
		ids := map[string]string{}
		var key string
		for _, src := range []string{"tmdb", "tvdb", "musicbrainz", "imdb"} {
			if v := clip(e.IDs[src], 60); v != "" {
				ids[src] = v
				if key == "" {
					key = src + ":" + v
				}
			}
		}
		if kind == "" || key == "" || (kind == "movie" && ids["tmdb"] == "") {
			b.not(GroupNeverAdd, from, e.Name, "no id HolyArrs can match")
			continue
		}
		b.add(GroupNeverAdd, Item{Key: "never_add:" + kind + ":" + key, Name: e.Name, From: from, Detail: "kept off the Library",
			NeverAdd: &NeverAddPlan{Kind: kind, IDs: ids}})
	}
}
