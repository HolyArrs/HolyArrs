package migrate

import (
	"fmt"
	"path"
	"strings"
)

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

// titleCopy is where a planned title's copy came from: its read, and
// whether its profile aims for 2160p.
type titleCopy struct {
	snap  int64
	fourK bool
	idx   int    // the title's place in the titles group
	addr  string // the app's address ("" for a backup)
}

// titleDetail says where a title's folder is, its Keep up to date choice
// and its Release gate, for the plan's line.
func titleDetail(tp *TitlePlan, monitored bool, gateNote string) string {
	d := "uses the folder " + path.Join(tp.Library, tp.Folder) + " where it is"
	if !monitored {
		d += "; Keep up to date off"
	}
	return d + gateNote
}

// fourKPlanOf takes a movie copy's plan as the 4K copy of a merged title
// whose main copy is main. A 4K copy in the main copy's own folder (one
// folder holding both apps' files) is not placed: each Version needs a
// folder of its own, so the real run could not add it.
func fourKPlanOf(main, tp *TitlePlan, from string) *FourKPlan {
	fk := &FourKPlan{Library: tp.Library, Folder: tp.Folder, Preset: tp.Preset, KeepUpToDate: tp.KeepUpToDate,
		Qualities: tp.Qualities, Unresolved: tp.Unresolved, From: from}
	if fk.Unresolved == "" && main.Unresolved == "" && main.Folder != "" && main.Library == fk.Library && strings.EqualFold(main.Folder, fk.Folder) {
		fk.Unresolved = "it shares the folder " + path.Join(main.Library, main.Folder) + " with the main copy, and a 4K Version needs a folder of its own"
	}
	return fk
}

// fourKNote says where a merged movie's 4K copy goes, for the title's line.
func fourKNote(fk *FourKPlan) string {
	if fk.Unresolved != "" {
		return "; its 4K copy is not placed: " + fk.Unresolved
	}
	return "; its 4K copy uses the folder " + path.Join(fk.Library, fk.Folder) + " where it is"
}

// mergeFourK folds a second Radarr's copy of an already planned movie into
// one title: when exactly one copy's profile aims for 2160p and the copies
// come from different reads, the other copy stays the main title and the
// 4K copy becomes its FourK plan. It says whether it merged; any other
// duplicate keeps the "imported once" line.
func (b *builder) mergeFourK(key string, tp *TitlePlan, from string, t Title, gateNote string, gated *int) bool {
	newFourK := b.fourK[b.presetKey(b.app, t.ProfileID)]
	src, ok := b.titleSrc[key]
	// Two reads of one app at one address are not two apps.
	if !ok || src.snap == b.snapID || src.fourK == newFourK || src.addr != "" && src.addr == b.addr {
		return false
	}
	items := b.items[GroupTitles].Items
	if src.idx < 0 || src.idx >= len(items) || items[src.idx].Key != key {
		return false
	}
	it := &items[src.idx]
	if it.Title == nil || it.Title.FourK != nil {
		return false
	}
	if newFourK {
		// The 4K read came second: the planned title keeps its place.
		fk := fourKPlanOf(it.Title, tp, from)
		it.Title.FourK = fk
		b.p.Mapped = append(b.p.Mapped, from)
		it.Detail += fourKNote(fk)
		return true
	}
	// The 4K read came first: this copy becomes the title. An unplaced
	// copy keeps today's not-placed line instead.
	if tp.Unresolved != "" {
		return false
	}
	fk := fourKPlanOf(tp, it.Title, it.From)
	tp.FourK = fk
	b.p.Mapped = append(b.p.Mapped, it.From)
	name := clip(t.Name, maxName)
	if t.Year > 0 {
		name = fmt.Sprintf("%s (%d)", name, t.Year)
	}
	// The earlier read counted its own title already; only this read's
	// count changes.
	if tp.GateDelayDays != nil {
		*gated++
	}
	it.Name, it.From, it.Title = name, from, tp
	it.Detail = titleDetail(tp, t.Monitored, gateNote) + fourKNote(fk)
	b.titleSrc[key] = titleCopy{snap: b.snapID, idx: src.idx, addr: b.addr}
	return true
}

// titlePlaces lists where a planned title's copies are, as Library folder
// and folder pairs: its own, and its 4K copy's when a second app's 4K copy
// merged into it. A 4K app's folder can hold only such merged copies.
func titlePlaces(t *TitlePlan) [][2]string {
	var out [][2]string
	if t == nil {
		return nil
	}
	if t.Folder != "" {
		out = append(out, [2]string{t.Library, t.Folder})
	}
	if t.FourK != nil && t.FourK.Folder != "" {
		out = append(out, [2]string{t.FourK.Library, t.FourK.Folder})
	}
	return out
}
