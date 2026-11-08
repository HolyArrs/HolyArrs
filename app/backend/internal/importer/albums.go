package importer

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/tags"
)

// Track is one audio file of an album with its tag facts.
type Track struct {
	ItemID int64
	Disc   int
	Number int
	Name   string
	SlotID int64
	Wanted bool
}

// AlbumTarget describes the album a download is placed into.
type AlbumTarget struct {
	LibraryPath  string
	ArtistFolder string
	Album        string
	Year         int
	Tracks       []Track
	Artist       string
	AlbumType    string
	Edition      string
	Naming       *mediafiles.NamingSettings
	KeepNames    bool
	Tags         func(path string) (tags.Tags, error)
}

var audioExt = map[string]bool{".flac": true, ".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".wav": true,
	".wv": true, ".ape": true, ".aiff": true, ".aif": true, ".alac": true, ".dsf": true, ".dff": true, ".wma": true, ".mka": true, ".tak": true}

// IsAudio reports whether ext is a known audio extension.
func IsAudio(ext string) bool { return audioExt[strings.ToLower(ext)] }

var (
	discDir = regexp.MustCompile(`(?i)^(?:cd|disc|disk)\s*[-_.]?\s*(\d{1,2})\b`)
	leadNum = regexp.MustCompile(`^\s*(?:(\d{1,2})[-.](\d{1,3})|(\d{1,3}))(?:[\s._)\]-]|$)`)
	midNum  = regexp.MustCompile(`\s-\s(\d{1,3})\s*[-.]\s`)
)

func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (t AlbumTarget) trackFile(tr Track, ext string, discs int, src string) string {
	ns := mediafiles.NamingDefaults()
	if t.Naming != nil {
		ns = *t.Naming
	}
	f := mediafiles.TrackFacts{Artist: t.Artist, Album: t.Album, AlbumType: t.AlbumType, Edition: t.Edition, Year: t.Year,
		Disc: tr.Disc, Discs: discs, Track: tr.Number, Title: tr.Name, Ext: ext}
	if t.KeepNames && src != "" {
		base := filepath.Base(src)
		own := cutName(mediafiles.CleanWith(ns, strings.TrimSuffix(base, filepath.Ext(base))), maxNameBytes-len(ext))
		if own != "" {
			return path.Join(t.ArtistFolder, mediafiles.AlbumFolder(ns, f), mediafiles.DiscFolder(ns, f), own+strings.ToLower(ext))
		}
	}
	return mediafiles.TrackPath(ns, t.ArtistFolder, f)
}

// maxNameBytes keeps a file name under the 255 byte limit of common filesystems, with room for a suffix.
const maxNameBytes = 240

func cutName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return strings.TrimRight(s[:n], ". ")
}

func discOf(p string) int {
	dirs := strings.Split(filepath.ToSlash(filepath.Dir(p)), "/")
	for i := len(dirs) - 1; i >= 0; i-- {
		if m := discDir.FindStringSubmatch(dirs[i]); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n
		}
	}
	return 0
}

func numberOf(base string, discs int) (disc, num int) {
	if m := leadNum.FindStringSubmatch(base); m != nil {
		if m[1] != "" {
			disc, _ = strconv.Atoi(m[1])
			num, _ = strconv.Atoi(m[2])
			return disc, num
		}
		num, _ = strconv.Atoi(m[3])
		if discs > 1 && num >= 101 && len(m[3]) == 3 {
			return num / 100, num % 100
		}
		return 0, num
	}
	if m := midNum.FindStringSubmatch(base); m != nil {
		num, _ = strconv.Atoi(m[1])
	}
	return 0, num
}

// PlanTracks turns candidate files into copy steps for an album.
func PlanTracks(cands []Candidate, t AlbumTarget) []mediafiles.PlanStep {
	discs := 1
	for _, tr := range t.Tracks {
		discs = max(discs, tr.Disc)
	}
	find := func(disc, num int) *Track {
		for i := range t.Tracks {
			if t.Tracks[i].Number == num && (t.Tracks[i].Disc == disc || disc == 0 && discs == 1) {
				return &t.Tracks[i]
			}
		}
		return nil
	}
	byName := func(base string, disc int) (*Track, []*Track) {
		var hit *Track
		var same []*Track
		fb := fold(base)
		for i := range t.Tracks {
			n := fold(t.Tracks[i].Name)
			if len(n) < 3 || !strings.Contains(fb, n) || disc != 0 && t.Tracks[i].Disc != disc {
				continue
			}
			if hit != nil && fold(hit.Name) == n {
				same = append(same, &t.Tracks[i])
				continue
			}
			if hit != nil && len(fold(hit.Name)) >= len(n) {
				continue
			}
			hit, same = &t.Tracks[i], []*Track{&t.Tracks[i]}
		}
		if len(same) > 1 {
			return nil, same
		}
		return hit, nil
	}
	var steps []mediafiles.PlanStep
	for _, c := range cands {
		ext := strings.ToLower(filepath.Ext(c.Path))
		base := strings.TrimSuffix(filepath.Base(c.Path), filepath.Ext(c.Path))
		s := mediafiles.PlanStep{SourceFile: c.Path, TargetSlotIDs: []int64{}, Why: []string{}}
		switch {
		case programExt[ext]:
			s.Method, s.Why = "refuse", []string{"a program file, not music"}
		case !audioExt[ext]:
			s.Method, s.Why = "skip", []string{"not an audio file (" + ext + "); left alone"}
		default:
			disc, num := numberOf(base, discs)
			if d := discOf(c.Path); d > 0 && disc == 0 {
				disc = d
			}
			var tr *Track
			why := ""
			if num > 0 {
				if tr = find(disc, num); tr != nil {
					why = "track number " + strconv.Itoa(num)
					if disc > 0 {
						why = "disc " + strconv.Itoa(disc) + ", " + why
					}
					if n := fold(tr.Name); n != "" && !strings.Contains(fold(base), n) {
						if other, _ := byName(base, disc); other != nil && other != tr {
							tr, why = nil, "the number says one track and the name another"
						}
					}
				}
			}
			var same []*Track
			if tr == nil && why == "" {
				if tr, same = byName(base, disc); tr != nil {
					why = "the track name is in the file name"
				}
			}
			if tr == nil && t.Tags != nil {
				if hit, how := t.byTags(c.Path, base, disc, discs, same, find, why == "" && num == 0 && len(same) == 0); hit != nil {
					tr, same, why = hit, nil, how
				}
			}
			switch {
			case len(same) > 1:
				labels := []string{}
				for _, o := range same {
					labels = append(labels, trackLabel(o.Disc, o.Number, discs))
					if o.Wanted {
						s.TargetSlotIDs = append(s.TargetSlotIDs, o.SlotID)
					}
				}
				s.Method, s.Why = "skip", []string{"more than one track has this name (" + strings.Join(labels, ", ") + "); which one it is needs a decision"}
			case tr == nil && why != "":
				s.Method, s.Why = "skip", []string{why + "; left alone"}
			case tr == nil && num > 0:
				s.Method, s.Why = "skip", []string{"no such track on the chosen pressing (a bonus track?); left alone"}
			case tr == nil:
				s.Method, s.Why = "skip", []string{"the file name gives no track number or track name; left alone"}
			case !tr.Wanted:
				s.Method, s.Why = "skip", []string{trackLabel(tr.Disc, tr.Number, discs) + " is not filled by this download; left alone"}
			default:
				s.Method, s.Confidence = "hardlink", 1
				s.TargetSlotIDs = []int64{tr.SlotID}
				s.TargetPath = filepath.Join(t.LibraryPath, filepath.FromSlash(t.trackFile(*tr, ext, discs, c.Path)))
				s.Why = []string{why, "holds " + trackLabel(tr.Disc, tr.Number, discs)}
			}
		}
		steps = append(steps, s)
	}
	order := []int{}
	for i := range steps {
		if steps[i].Method == "hardlink" {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		return sizeOf(cands, steps[order[a]].SourceFile) > sizeOf(cands, steps[order[b]].SourceFile)
	})
	taken := map[int64]bool{}
	for _, i := range order {
		id := steps[i].TargetSlotIDs[0]
		if taken[id] {
			steps[i].Method, steps[i].TargetPath = "skip", ""
			steps[i].Why = append(steps[i].Why, "a bigger file in the same download holds the same track")
			continue
		}
		taken[id] = true
	}
	return steps
}

func (t AlbumTarget) byTags(p, base string, disc, discs int, same []*Track, find func(int, int) *Track, bare bool) (*Track, string) {
	tg, err := t.Tags(p)
	if err != nil || tg.Track == 0 {
		return nil, ""
	}
	d := tg.Disc
	if d == 0 {
		d = disc
	}
	hit := find(d, tg.Track)
	if hit == nil && d == 0 {
		for i := range t.Tracks {
			if t.Tracks[i].Number == tg.Track {
				if hit != nil {
					return nil, ""
				}
				hit = &t.Tracks[i]
			}
		}
	}
	if hit == nil {
		return nil, ""
	}
	n := fold(hit.Name)
	ok := false
	switch {
	case tg.Title != "":
		ok = fold(tg.Title) == n
	default:
		ok = slices.Contains(same, hit) || bare || n != "" && strings.Contains(fold(base), n)
	}
	if !ok {
		return nil, ""
	}
	return hit, "the file's tags give " + trackLabel(hit.Disc, hit.Number, discs)
}

func trackLabel(disc, num, discs int) string {
	if discs > 1 {
		return fmt.Sprintf("disc %d track %02d", disc, num)
	}
	return fmt.Sprintf("track %02d", num)
}

func wholeAlbumSpace(plan []mediafiles.PlanStep, low *SpaceError) {
	if low == nil {
		return
	}
	for i := range plan {
		if plan[i].Method == "hardlink" {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append([]string{"not enough free space for the whole album"}, plan[i].Why...)
		}
	}
}

// AlbumFacts loads the naming facts and track count for one album part.
func AlbumFacts(ctx context.Context, db database.Querier, artist string, partID int64) (mediafiles.TrackFacts, int, error) {
	f := mediafiles.TrackFacts{Artist: artist}
	var p mediafiles.Part
	if err := database.Get(ctx, db, "parts", partID, &p); err != nil {
		return f, 1, err
	}
	f.Album = p.Name
	if p.ReleaseDate != nil {
		f.Year = p.ReleaseDate.Year()
	}
	if p.ReleaseType != nil {
		f.AlbumType = *p.ReleaseType
	}
	var dis string
	_ = db.QueryRowContext(ctx, "SELECT disambiguation FROM pressings WHERE part_id = ? AND chosen = 1 LIMIT 1", partID).Scan(&dis)
	f.Edition = dis
	if f.Edition == "" && p.Disambiguation != nil {
		f.Edition = *p.Disambiguation
	}
	var discs sql.NullInt64
	_ = db.QueryRowContext(ctx, `SELECT max(json_extract(numbers, '$.disc')) FROM items WHERE part_id = ? AND kind = 'track' AND dropped_at IS NULL`, partID).Scan(&discs)
	return f, max(1, int(discs.Int64)), nil
}

func albumTarget(ctx context.Context, db database.Querier, lf mediafiles.LibraryFolder, ver mediafiles.Version, ti mediafiles.Title, items []mediafiles.Item,
	slotOf map[int64]int64, slots map[int64]*showSlot, dryRun bool, ns mediafiles.NamingSettings) (AlbumTarget, error) {
	at := AlbumTarget{LibraryPath: lf.Path, ArtistFolder: safeName(ver.FolderName), Artist: ti.Name, Naming: &ns, KeepNames: !ns.RenameOnImport}
	if at.ArtistFolder == "" || at.ArtistFolder == "." || at.ArtistFolder == ".." {
		at.ArtistFolder = mediafiles.ArtistFolder(ns, mediafiles.TrackFacts{Artist: ti.Name})
	}
	if ns.MusicReadTags {
		at.Tags = tags.ReadFile
	}
	if len(items) > 0 && items[0].PartID != nil {
		f, _, err := AlbumFacts(ctx, db, ti.Name, *items[0].PartID)
		if err != nil {
			return at, err
		}
		at.Album, at.Year, at.AlbumType, at.Edition = f.Album, f.Year, f.AlbumType, f.Edition
	}
	for _, it := range items {
		tr := Track{ItemID: it.ID, Disc: 1, SlotID: slotOf[it.ID]}
		if it.Numbers.Disc != nil && *it.Numbers.Disc > 0 {
			tr.Disc = *it.Numbers.Disc
		}
		if it.Numbers.Track != nil {
			tr.Number = *it.Numbers.Track
		}
		if it.Name != nil {
			tr.Name = *it.Name
		}
		if ss := slots[tr.SlotID]; ss != nil && (ss.live || dryRun) {
			tr.Wanted = true
		}
		at.Tracks = append(at.Tracks, tr)
	}
	return at, nil
}
