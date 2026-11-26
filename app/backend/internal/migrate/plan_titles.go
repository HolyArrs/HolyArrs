package migrate

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
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

// unplacedCopy is a movie copy that could not be placed, kept so a copy of
// the same movie that a later read of the other kind of Radarr places can
// carry it as its 4K copy. note is its own not-moved line: another app's
// lines can have the same from.
type unplacedCopy struct {
	tp    *TitlePlan
	from  string
	snap  int64
	fourK bool
	addr  string
	note  NotMoved
}

// carryUnplacedFourK gives the movie just planned at idx the 4K copy of it
// that an earlier read could not place, as mergeFourK does when the 4K
// read comes second: the copy rides along with its reason instead of a
// not-moved line, so the main copy still keeps its own app's file. An
// unplaced 1080p copy keeps its not-placed line, as before.
func (b *builder) carryUnplacedFourK(key string, idx int, fourK bool) {
	u, ok := b.unplaced[key]
	if !ok || !u.fourK || fourK || u.snap == b.snapID || u.addr != "" && u.addr == b.addr {
		return
	}
	it := &b.items[GroupTitles].Items[idx]
	if it.Title == nil || it.Title.FourK != nil {
		return
	}
	fk := fourKPlanOf(it.Title, u.tp, u.from)
	it.Title.FourK = fk
	it.Detail += fourKNote(fk)
	b.p.Mapped = append(b.p.Mapped, u.from)
	if i := slices.Index(b.p.NotMoved, u.note); i >= 0 {
		b.p.NotMoved = slices.Delete(b.p.NotMoved, i, i+1)
	}
	delete(b.unplaced, key)
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
// folder holding both apps' files), or in a folder whose name a Version
// cannot keep, is not placed: the real run could not add it.
func fourKPlanOf(main, tp *TitlePlan, from string) *FourKPlan {
	fk := &FourKPlan{Library: tp.Library, Folder: tp.Folder, Preset: tp.Preset, KeepUpToDate: tp.KeepUpToDate,
		Qualities: tp.Qualities, Releases: tp.Releases, File: tp.File, Unresolved: tp.Unresolved, From: from}
	switch {
	case fk.Unresolved != "":
	case main.Unresolved == "" && main.Folder != "" && main.Library == fk.Library && foldName(main.Folder) == foldName(fk.Folder):
		fk.Unresolved = "it shares the folder " + path.Join(main.Library, main.Folder) + " with the main copy, and a 4K Version needs a folder of its own; put its 4K file in a folder of its own, then add it as a 4K Version on the title page"
	case !library.ExistingVersionFolderOK(fk.Folder):
		fk.Unresolved = "its folder name " + strconv.Quote(fk.Folder) + " cannot be a Version's folder"
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

// fourKClashes leaves out a merged movie's 4K copy whose folder another
// planned title also uses, as its own folder or as its 4K copy's: adding
// a Version refuses a folder that is taken, so the real run could not add
// it, whatever order the titles come in. Folders match without regard to
// case, as that check does. Titles' own folders are never changed.
func (b *builder) fourKClashes() {
	g := b.items[GroupTitles]
	if g == nil {
		return
	}
	key := func(lib, folder string) string { return lib + "\x00" + foldName(folder) }
	users := map[string][]int{}
	for i, it := range g.Items {
		t := it.Title
		if t == nil {
			continue
		}
		if t.Folder != "" && t.Unresolved == "" {
			users[key(t.Library, t.Folder)] = append(users[key(t.Library, t.Folder)], i)
		}
		if fk := t.FourK; fk != nil && fk.Folder != "" && fk.Unresolved == "" {
			users[key(fk.Library, fk.Folder)] = append(users[key(fk.Library, fk.Folder)], i)
		}
	}
	for i := range g.Items {
		it := &g.Items[i]
		if it.Title == nil || it.Title.FourK == nil || it.Title.FourK.Unresolved != "" || it.Title.FourK.Folder == "" {
			continue
		}
		fk := it.Title.FourK
		for _, j := range users[key(fk.Library, fk.Folder)] {
			if j != i {
				before := fourKNote(fk)
				fk.Unresolved = "its folder " + path.Join(fk.Library, fk.Folder) + " is also used by " + g.Items[j].Name
				it.Detail = strings.Replace(it.Detail, before, fourKNote(fk), 1)
				break
			}
		}
	}
}

// dropUnmergedFiles keeps a title's recorded file name in the plan only
// for a merged movie, the one place the run uses it, so an ordinary
// title's plan, and the hash an approved import was checked against, stay
// as they were.
func (b *builder) dropUnmergedFiles() {
	if g := b.items[GroupTitles]; g != nil {
		for i := range g.Items {
			if t := g.Items[i].Title; t != nil && t.FourK == nil {
				t.File = ""
			}
		}
	}
}

// foldName lowers a folder name as SQLite's lower() does, letters A to Z
// only: the way adding a Version compares folders, so the plan and the dry
// run match the real run (on Linux "Ä" and "ä" are two folders).
func foldName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, s)
}
