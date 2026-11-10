package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Errors about adopting music.
var (
	ErrNotMusicSource = errors.New("music details (MusicBrainz) are not set up; set them up before scanning a music Library folder")
)

// maxArtistFiles is the most audio files one artist folder scan reads.
const maxArtistFiles = 5000

// maxAlbumFileDetail is the most files a scan lists for one album folder
// (and loose in one artist folder); the counts still cover every file.
const maxAlbumFileDetail = 40

// busyWait is how long a scan waits when MusicBrainz lookups are queued
// full before it asks again.
var busyWait = 2 * time.Second

// mbRetry runs a MusicBrainz lookup, waiting while the shared queue is full
// (a scan is a background lookup and gives way to everyone else).
func mbRetry[T any](ctx context.Context, f func(context.Context) (T, error)) (T, error) {
	for {
		v, err := f(ctx)
		if !errors.Is(err, metadata.ErrMusicBusy) {
			return v, err
		}
		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-time.After(busyWait):
		}
	}
}

// AdoptTrackFile is an audio file in an album folder and the track it holds;
// Why says why it holds none (it is left alone).
type AdoptTrackFile struct {
	Path  string `json:"path"` // relative to the Library folder
	Size  int64  `json:"size"`
	Track string `json:"track,omitempty"` // "track 03 Title" or "disc 2 track 01 Title"
	Why   string `json:"why,omitempty"`
	slot  int64
}

// AdoptAlbum is one album folder in an artist folder and the album it may
// be, with the Pressing its files fit and the track each file holds.
type AdoptAlbum struct {
	Folder     string           `json:"folder"` // relative to the artist folder
	ParsedName string           `json:"parsed_name"`
	ParsedYear int              `json:"parsed_year,omitempty"`
	Match      *AdoptMatch      `json:"match,omitempty"` // ID is the album's MusicBrainz id
	Pressing   string           `json:"pressing,omitempty"`
	Tracks     int              `json:"tracks,omitempty"` // tracks on that Pressing
	Files      []AdoptTrackFile `json:"files"`
	Matched    int              `json:"matched"`
	Unknown    int              `json:"unknown"`
	// Cut says Files lists only the first files (Matched and Unknown count
	// them all).
	Cut bool `json:"cut,omitempty"`
	// Status: matched, unsure, unmatched, not_included.
	Status string `json:"status"`
	Why    string `json:"why"`
}

// albumRef is an album a folder may be: from MusicBrainz in a scan, from
// the Library in an adopt.
type albumRef struct {
	ID       string
	Name     string
	Year     int
	PartID   int64
	Included bool
}

// albumDir is an album folder with its audio files.
type albumDir struct {
	name  string
	files []AdoptVideo
}

var reYearFirst = regexp.MustCompile(`^[(\[]?((?:18|19|20)\d{2})[)\]]?\s*[-._ ]+\s*(.+)$`)

// albumFolderName reads an album folder name: "Album (Year)", "Year -
// Album", "Artist - Album (Year)" and the like.
func albumFolderName(folder, artist string) (string, int) {
	s := strings.TrimSpace(folder)
	if a := strings.TrimSpace(artist); a != "" && len(s) > len(a) && strings.EqualFold(s[:len(a)], a) {
		if rest := strings.TrimLeft(s[len(a):], " -_."); rest != "" && rest != s[len(a):] {
			s = rest
		}
	}
	if m := reYearFirst.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		return strings.TrimSpace(m[2]), y
	}
	return FolderName(s)
}

// artistScore says how well an artist found fits a folder name (0..1).
func artistScore(folder string, a metadata.Artist) float64 {
	s := max(wordScore(folder, a.Name), wordScore(folder, a.SortName), wordScore(folder, theLastName(a.Name)))
	return float64(int(s*100+0.5)) / 100
}

// theLastName is "Beatles, The" for "The Beatles".
func theLastName(n string) string {
	if rest, ok := strings.CutPrefix(n, "The "); ok {
		return rest + ", The"
	}
	return n
}

// listAudio finds the audio files in one folder of a Library folder (its
// disc folders too), biggest first, at most limit. Links are never followed.
func listAudio(lib, folder string, limit int) []AdoptVideo {
	root := filepath.Join(lib, folder)
	var out []AdoptVideo
	_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if e.IsDir() {
			if p != root && (strings.HasPrefix(e.Name(), ".") || NasSystemDir(e.Name()) || strings.Count(rel, string(filepath.Separator)) >= maxScanDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		linked, isLink := importer.LinkedFile(p)
		if (e.Type()&fs.ModeSymlink != 0 && !isLink) || (!isLink && !e.Type().IsRegular()) || !importer.IsAudio(filepath.Ext(p)) || len(out) >= limit {
			return nil
		}
		fi, err := e.Info()
		if isLink {
			fi, err = linked, nil
		}
		if err != nil {
			return nil
		}
		out = append(out, AdoptVideo{Path: path.Join(filepath.ToSlash(folder), filepath.ToSlash(rel)), Size: fi.Size()})
		return nil
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

// albumDirs splits an artist folder's audio files into its album folders;
// files directly in the artist folder are loose.
func albumDirs(artistFolder string, files []AdoptVideo) (dirs []albumDir, loose []AdoptVideo) {
	idx := map[string]int{}
	for _, f := range files {
		rel := strings.TrimPrefix(f.Path, artistFolder+"/")
		top, _, nested := strings.Cut(rel, "/")
		if !nested {
			loose = append(loose, f)
			continue
		}
		i, ok := idx[top]
		if !ok {
			i = len(dirs)
			idx[top] = i
			dirs = append(dirs, albumDir{name: top})
		}
		dirs[i].files = append(dirs[i].files, f)
	}
	sort.SliceStable(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	return dirs, loose
}

// matchAlbums gives each album folder the album that fits it best, each
// album to one folder at most (the best-fitting folder wins it). The
// second result holds the score of the runner-up album, to tell close
// calls.
func matchAlbums(dirs []albumDir, artist string, refs []albumRef) ([]*albumRef, []float64, []float64) {
	type pair struct {
		d, r int
		s    float64
	}
	var pairs []pair
	scores := make([]float64, len(dirs))
	second := make([]float64, len(dirs))
	for i, d := range dirs {
		name, year := albumFolderName(d.name, artist)
		for j, r := range refs {
			sc := NameScore(name, year, r.Name, r.Year)
			if sc >= MatchUnsure {
				pairs = append(pairs, pair{i, j, sc})
			}
		}
	}
	sort.SliceStable(pairs, func(a, b int) bool { return pairs[a].s > pairs[b].s })
	out := make([]*albumRef, len(dirs))
	used := map[int]bool{}
	for _, p := range pairs {
		switch {
		case out[p.d] == nil && !used[p.r]:
			out[p.d], scores[p.d] = &refs[p.r], p.s
			used[p.r] = true
		case out[p.d] != nil && second[p.d] == 0 && refs[p.r].ID != out[p.d].ID:
			second[p.d] = p.s
		}
	}
	return out, scores, second
}

// pickPressing chooses the Pressing whose track count is the number of
// audio files (by the Pressing rule among those), else by the rule alone.
func pickPressing(ps []domain.AlbumPressing, files int, statuses []string, rule string) int {
	var fit []domain.AlbumPressing
	var at []int
	for i, p := range ps {
		if p.TrackCount == files {
			fit, at = append(fit, p), append(at, i)
		}
	}
	if i := ChoosePressing(fit, statuses, rule); i >= 0 {
		return at[i]
	}
	return ChoosePressing(ps, statuses, rule)
}

// mapTrackFiles matches the audio files of an album folder to tracks with
// the album import's own matching (numbers, CD folders, names, and tags
// when names do not settle it). Tracks carry their Slot (in a scan, a
// stand-in id). The biggest file of a track wins it.
func mapTrackFiles(lib string, files []AdoptVideo, tracks []importer.Track, ns mediafiles.NamingSettings) ([]AdoptTrackFile, int) {
	cands := make([]importer.Candidate, len(files))
	for i, f := range files {
		cands[i] = importer.Candidate{Path: filepath.Join(lib, filepath.FromSlash(f.Path)), Size: f.Size}
	}
	at := importer.AlbumTarget{LibraryPath: lib, ArtistFolder: "a", Album: "a", Tracks: tracks, Naming: &ns}
	if ns.MusicReadTags {
		at.Tags = importer.ReadFileTags
	}
	discs := 1
	label := map[int64]string{}
	for _, tr := range tracks {
		discs = max(discs, tr.Disc)
	}
	for _, tr := range tracks {
		l := fmt.Sprintf("track %02d", tr.Number)
		if discs > 1 {
			l = fmt.Sprintf("disc %d track %02d", tr.Disc, tr.Number)
		}
		label[tr.SlotID] = strings.TrimSpace(l + " " + tr.Name)
	}
	steps := importer.PlanTracks(cands, at)
	out := make([]AdoptTrackFile, 0, len(files))
	matched := 0
	for i, st := range steps {
		f := AdoptTrackFile{Path: files[i].Path, Size: files[i].Size}
		if st.Method == "hardlink" && len(st.TargetSlotIDs) == 1 {
			f.slot, f.Track = st.TargetSlotIDs[0], label[st.TargetSlotIDs[0]]
			matched++
		} else {
			f.Why = strings.Join(st.Why, "; ")
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, matched
}

// pressingTracks turns a Pressing's tracks into album-import tracks with
// stand-in Slots (1, 2, ...).
func pressingTracks(p domain.AlbumPressing) []importer.Track {
	out := make([]importer.Track, len(p.Tracks))
	for i, tr := range p.Tracks {
		out[i] = importer.Track{ItemID: int64(i + 1), Disc: max(1, tr.Disc), Number: tr.Number, Name: tr.Name, SlotID: int64(i + 1), Wanted: true}
	}
	return out
}

func fromMetadata(mp metadata.Pressing) domain.AlbumPressing {
	p := domain.AlbumPressing{MBID: mp.MBID, Name: mp.Name, Status: mp.Status, Country: mp.Country, Date: mp.Date, Disambiguation: mp.Disambiguation,
		Formats: mp.Formats, TrackCount: len(mp.Tracks)}
	for _, tr := range mp.Tracks {
		p.Tracks = append(p.Tracks, domain.PressingTrack{ID: tr.ID, RecordingID: tr.RecordingID, Disc: tr.Disc, Number: tr.Number, Position: tr.Position, Name: tr.Name})
	}
	return p
}

// cutAlbumFiles keeps the first maxAlbumFileDetail files of an album.
func cutAlbumFiles(al *AdoptAlbum) {
	if len(al.Files) > maxAlbumFileDetail {
		al.Files, al.Cut = al.Files[:maxAlbumFileDetail], true
	}
}

// PlanMusicAdoptFrom scans a music Library folder: each folder in it is
// matched to a MusicBrainz artist with a confidence score, its album
// folders to that artist's albums, and each album's files to the tracks of
// the Pressing they fit. Nothing is written. At most the music scan limit
// of artist folders are read; its lookups are background lookups that
// wait behind imports, searches and staff.
func PlanMusicAdoptFrom(ctx context.Context, db Reader, artists metadata.ArtistProvider, folderID int64, after string) (AdoptPlan, error) {
	return planMusicAdopt(ctx, db, artists, folderID, after, nil, nil)
}

// planMusicAdopt is PlanMusicAdoptFrom that goes on from seed (the part a
// stopped scan finished) and hands save the plan so far after each artist
// folder (More and Next set, Partial on).
func planMusicAdopt(ctx context.Context, db Reader, artists metadata.ArtistProvider, folderID int64, after string, seed *AdoptPlan,
	save func(AdoptPlan) error) (AdoptPlan, error) {
	ctx = metadata.MemberLookup(ctx)
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", folderID, &lf); err != nil {
		return AdoptPlan{}, ErrNoFolder
	}
	if lf.Kind != "music" {
		return AdoptPlan{}, ErrNotMusicFolder
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return AdoptPlan{}, err
	}
	defs, err := ReadMusicDefaults(ctx, db)
	if err != nil {
		return AdoptPlan{}, err
	}
	p := AdoptPlan{Kind: "music", LibraryFolderID: lf.ID, Path: lf.Path, Folders: []AdoptCandidate{}, Counts: map[string]int{}, Limit: ns.MusicAdoptScanLimit}
	if seed != nil && seed.LibraryFolderID == lf.ID {
		p.Folders = append(p.Folders, seed.Folders...)
		for k, v := range seed.Counts {
			p.Counts[k] = v
		}
		if seed.Next != "" {
			after = seed.Next
		}
	}
	if artists == nil {
		return p, ErrNotMusicSource
	}
	ents, err := os.ReadDir(lf.Path)
	if err != nil {
		return p, fmt.Errorf("the Library folder cannot be read: %w", err)
	}
	known := map[string]int64{}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "library_folder_id = ? AND title_id IN (SELECT id FROM titles WHERE state != 'removed')", lf.ID)
	if err != nil {
		return p, err
	}
	fold := FoldsCase(lf.Path)
	for _, v := range vs {
		known[FolderKey(v.FolderName, fold)] = v.TitleID
	}
	p.Warning = MountWarning(ctx, db, lf, len(ents))
	var saveErr error
	add := func(c AdoptCandidate) {
		if len(c.Files) > maxAlbumFileDetail {
			c.Files, c.Cut = c.Files[:maxAlbumFileDetail], true
		}
		p.Folders = append(p.Folders, c)
		p.Counts[c.Status]++
		if save != nil && saveErr == nil {
			part := p
			part.More, part.Next, part.Partial = true, c.Folder, true
			saveErr = save(part)
		}
	}
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || NasSystemDir(e.Name()) || e.Name() == importer.HoldingDir || (after != "" && e.Name() <= after) {
			continue
		}
		if len(p.Folders) >= p.Limit {
			p.More, p.Next = true, p.Folders[len(p.Folders)-1].Folder
			break
		}
		c := AdoptCandidate{Folder: e.Name(), ParsedName: e.Name(), Videos: []AdoptVideo{}, Others: []AdoptMatch{}, Albums: []AdoptAlbum{}}
		if id, ok := known[FolderKey(e.Name(), fold)]; ok {
			c.Status, c.Why, c.TitleID = "in_library", "this folder already belongs to an artist in the Library", id
			add(c)
			continue
		}
		if saveErr != nil {
			return p, saveErr
		}
		found, err := mbRetry(ctx, func(ctx context.Context) ([]metadata.Artist, error) { return artists.SearchArtists(ctx, e.Name()) })
		if err != nil {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			c.Status, c.Why = "unmatched", "the MusicBrainz lookup failed ("+err.Error()+"); look it up by hand or scan again later"
			add(c)
			continue
		}
		var ms []AdoptMatch
		for _, a := range found {
			ms = append(ms, AdoptMatch{ID: "mbid:" + a.MBID, Name: a.Name, Score: artistScore(e.Name(), a)})
		}
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Score > ms[j].Score })
		if len(ms) > 4 {
			ms = ms[:4]
		}
		if len(ms) > 0 && ms[0].Score >= MatchUnsure {
			c.Match = &ms[0]
			c.Others = append(c.Others, ms[1:]...)
		} else {
			c.Others = append(c.Others, ms...)
		}
		if c.Match == nil {
			c.Status, c.Why = "unmatched", "no artist at MusicBrainz fits this folder name; pick one by hand"
			add(c)
			continue
		}
		mbid := strings.TrimPrefix(c.Match.ID, "mbid:")
		a, err := mbRetry(ctx, func(ctx context.Context) (metadata.Artist, error) { return artists.Artist(ctx, mbid) })
		if err != nil {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			c.Status, c.Why = "unmatched", "the album list could not be read ("+err.Error()+"); scan again later"
			add(c)
			continue
		}
		kinds := mediafiles.Title{MusicTypes: defs.AlbumTypes, MusicSecondary: defs.SecondaryTypes}
		var refs []albumRef
		for _, al := range a.Albums {
			r := albumRef{ID: al.MBID, Name: al.Name, Included: AlbumKindsOK(kinds, al.PrimaryType, al.SecondaryTypes)}
			if d := musicDate(al.ReleaseDate); d != nil {
				r.Year = d.Year()
			}
			refs = append(refs, r)
		}
		files := listAudio(lf.Path, e.Name(), maxArtistFiles)
		dirs, loose := albumDirs(e.Name(), files)
		got, scores, second := matchAlbums(dirs, a.Name, refs)
		for i, d := range dirs {
			al := AdoptAlbum{Folder: d.name, Files: []AdoptTrackFile{}}
			al.ParsedName, al.ParsedYear = albumFolderName(d.name, a.Name)
			r := got[i]
			if r == nil {
				al.Status, al.Why, al.Unknown = "unmatched", "no album of this artist fits this folder name; its files are left alone", len(d.files)
				for _, f := range d.files {
					al.Files = append(al.Files, AdoptTrackFile{Path: f.Path, Size: f.Size, Why: "the album folder matches no album"})
				}
				cutAlbumFiles(&al)
				c.Albums = append(c.Albums, al)
				c.Unknown += al.Unknown
				continue
			}
			al.Match = &AdoptMatch{ID: r.ID, Name: r.Name, Year: r.Year, Score: scores[i]}
			if !r.Included {
				al.Status, al.Unknown = "not_included", len(d.files)
				al.Why = r.Name + " is left out by the album kinds in Settings > Music, so its files are left alone"
				for _, f := range d.files {
					al.Files = append(al.Files, AdoptTrackFile{Path: f.Path, Size: f.Size, Why: "the album is left out by the album kinds"})
				}
				cutAlbumFiles(&al)
				c.Albums = append(c.Albums, al)
				c.Unknown += al.Unknown
				continue
			}
			mps, perr := mbRetry(ctx, func(ctx context.Context) ([]metadata.Pressing, error) { return artists.Pressings(ctx, r.ID) })
			if perr != nil && !errors.Is(perr, metadata.ErrListCut) || len(mps) == 0 {
				if ctx.Err() != nil {
					return p, ctx.Err()
				}
				al.Status, al.Why, al.Unknown = "unsure", "the album's Pressings could not be read; the files are matched when adopted", len(d.files)
				cutAlbumFiles(&al)
				c.Albums = append(c.Albums, al)
				continue
			}
			ps := make([]domain.AlbumPressing, len(mps))
			for j, mp := range mps {
				ps[j] = fromMetadata(mp)
			}
			pr := ps[pickPressing(ps, len(d.files), defs.ReleaseStatuses, defs.PressingRule)]
			al.Pressing, al.Tracks = pressingLabel(pr), pr.TrackCount
			al.Files, al.Matched = mapTrackFiles(lf.Path, d.files, pressingTracks(pr), ns)
			al.Unknown = len(al.Files) - al.Matched
			label := r.Name
			if r.Year > 0 {
				label += fmt.Sprintf(" (%d)", r.Year)
			}
			close := second[i] > scores[i]-0.1
			if scores[i] < MatchSure || close {
				al.Status, al.Why = "unsure", fmt.Sprintf("best match %s at %.0f%%; check it", label, scores[i]*100)
			} else {
				al.Status, al.Why = "matched", fmt.Sprintf("%s at %.0f%%: %d of %d files hold a track of %d", label, scores[i]*100, al.Matched, len(al.Files), pr.TrackCount)
			}
			cutAlbumFiles(&al)
			c.Albums = append(c.Albums, al)
			c.Tracks += al.Matched
			c.Unknown += al.Unknown
		}
		for _, f := range loose {
			c.Unknown++
			c.Files = append(c.Files, AdoptEpisodeFile{Path: f.Path, Size: f.Size, Episodes: []string{}, Why: "not in an album folder; left alone"})
		}
		inLib, err := ArtistByMBID(ctx, db, mbid)
		if err != nil {
			return p, err
		}
		never, err := ArtistOnNeverAdd(ctx, db, mbid)
		if err != nil {
			return p, err
		}
		close := len(ms) > 1 && ms[1].Score > c.Match.Score-0.1
		switch {
		case inLib != 0:
			c.Status, c.Why, c.TitleID = "in_library", "this artist is already in the Library in another folder", inLib
		case never:
			c.Status, c.Why = "never_add", "this artist is on the Never-add list"
		case len(files) == 0:
			c.Status, c.Why = "no_video", "no audio file in the folder; adopting adds the artist with every album missing"
		case c.Match.Score < MatchSure || close:
			c.Status, c.Why = "unsure", fmt.Sprintf("best match %s at %.0f%%; check it", c.Match.Name, c.Match.Score*100)
		default:
			n := 0
			for _, al := range c.Albums {
				if al.Match != nil {
					n++
				}
			}
			c.Status, c.Why = "matched", fmt.Sprintf("%s at %.0f%%: %d tracks in %d albums, %d files left alone", c.Match.Name, c.Match.Score*100, c.Tracks, n, c.Unknown)
		}
		add(c)
	}
	return p, saveErr
}

// MusicAdoption is an artist folder found on disk that the person
// confirmed: the folder is kept as it is and its audio files become
// adopted Files of the tracks they hold.
type MusicAdoption struct {
	FolderName string
	Adopted    int
	Albums     int
	Unknown    []string
}

// adoptArtistOne adds one artist folder as an artist with its folder and
// files as they are: the artist is added, its albums, Pressings and tracks
// are fetched, each album folder is matched to an album (the Pressing its
// files fit is chosen) and each file to a track. Nothing on disk changes.
func (s *Service) adoptArtistOne(ctx context.Context, lf mediafiles.LibraryFolder, c AdoptChoice, by domain.Actor) (AdoptResult, error) {
	r := AdoptResult{Folder: c.Folder}
	if c.Folder == "" || strings.ContainsAny(c.Folder, `/\`) || !filepath.IsLocal(c.Folder) || strings.HasPrefix(c.Folder, ".") || c.Folder == importer.HoldingDir {
		return r, errors.New("the folder must be one folder directly in the Library folder")
	}
	if fi, err := os.Lstat(filepath.Join(lf.Path, c.Folder)); err != nil || !fi.IsDir() {
		return r, errors.New("no such folder in the Library folder")
	}
	if taken, err := FolderTaken(ctx, s.Store.Read(), lf, c.Folder); err != nil {
		return r, err
	} else if taken {
		return r, errors.New("this folder already belongs to an artist in the Library")
	}
	a := &MusicAdoption{FolderName: c.Folder}
	id, _, err := s.AddArtist(ctx, c.ID, ArtistAddOptions{LibraryFolderID: lf.ID, WhyAdded: "import", Adopt: a}, by)
	if err != nil {
		return r, err
	}
	r.TitleID = id
	if _, err := s.RefreshArtist(useCache(ctx), id, by, nil); err != nil {
		return r, fmt.Errorf("the artist was added but its albums could not be read (%w); its files are recorded on the next rescan", err)
	}
	if err := s.adoptAlbums(ctx, lf, id, a, by); err != nil {
		return r, err
	}
	r.Episodes, r.Unknown = a.Adopted, a.Unknown
	var name string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT name FROM titles WHERE id = ?", id).Scan(&name)
	summary := fmt.Sprintf("Adopted %s from %s: %d tracks in %d albums", name, c.Folder, a.Adopted, a.Albums)
	if len(a.Unknown) > 0 {
		summary += fmt.Sprintf(", %d files left alone", len(a.Unknown))
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &id, Summary: summary,
		Data:     map[string]any{"library_folder_id": lf.ID, "folder": c.Folder, "id": c.ID, "files": a.Adopted, "albums": a.Albums, "left_alone": a.Unknown},
		GroupKey: GroupKey(id)})
	return r, nil
}

// adoptAlbums matches an adopted artist folder's album folders to the
// artist's albums in the Library and records each matched file as an
// adopted File of its track.
func (s *Service) adoptAlbums(ctx context.Context, lf mediafiles.LibraryFolder, titleID int64, a *MusicAdoption, by domain.Actor) error {
	db := s.Store.Read()
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", titleID, &t); err != nil {
		return err
	}
	var v mediafiles.Version
	if err := db.QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ? AND is_main = 1", titleID).Scan(&v.ID); err != nil {
		return err
	}
	if err := database.Get(ctx, db, "versions", v.ID, &v); err != nil {
		return err
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, db, "pick_rules", v.PickRulesID, &pr); err != nil {
		return err
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return err
	}
	defs, err := ReadMusicDefaults(ctx, db)
	if err != nil {
		return err
	}
	statuses := t.MusicStatuses
	if len(statuses) == 0 {
		statuses = defs.ReleaseStatuses
	}
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL ORDER BY id", titleID)
	if err != nil {
		return err
	}
	refs := make([]albumRef, len(parts))
	for i, p := range parts {
		refs[i] = albumRef{ID: p.SourceIDs["musicbrainz"], Name: p.Name, PartID: p.ID, Included: p.Included}
		if p.ReleaseDate != nil {
			refs[i].Year = p.ReleaseDate.Year()
		}
	}
	dirs, loose := albumDirs(v.FolderName, listAudio(lf.Path, v.FolderName, maxArtistFiles))
	for _, f := range loose {
		a.Unknown = append(a.Unknown, f.Path+": not in an album folder")
	}
	got, _, _ := matchAlbums(dirs, t.Name, refs)
	for i, d := range dirs {
		r := got[i]
		if r == nil {
			for _, f := range d.files {
				a.Unknown = append(a.Unknown, f.Path+": the album folder matches no album")
			}
			continue
		}
		if !r.Included {
			for _, f := range d.files {
				a.Unknown = append(a.Unknown, f.Path+": "+r.Name+" is not included by the artist's album kinds")
			}
			continue
		}
		ps, err := database.Select[domain.AlbumPressing](ctx, db, "pressings", "part_id = ? AND dropped_at IS NULL ORDER BY id", r.PartID)
		if err != nil {
			return err
		}
		if i := pickPressing(ps, len(d.files), statuses, defs.PressingRule); i >= 0 && !ps[i].Chosen {
			if _, err := s.SetAlbum(ctx, r.PartID, nil, ps[i].ID, by); err != nil {
				return err
			}
		}
		items, err := database.Select[mediafiles.Item](ctx, db, "items", "part_id = ? AND kind = 'track' AND dropped_at IS NULL ORDER BY id", r.PartID)
		if err != nil {
			return err
		}
		var tracks []importer.Track
		for _, it := range items {
			tr := importer.Track{ItemID: it.ID, Disc: 1, Wanted: true}
			if it.Numbers.Disc != nil && *it.Numbers.Disc > 0 {
				tr.Disc = *it.Numbers.Disc
			}
			if it.Numbers.Track != nil {
				tr.Number = *it.Numbers.Track
			}
			if it.Name != nil {
				tr.Name = *it.Name
			}
			if db.QueryRowContext(ctx, "SELECT id FROM slots WHERE item_id = ? AND version_id = ?", it.ID, v.ID).Scan(&tr.SlotID) != nil {
				continue
			}
			tracks = append(tracks, tr)
		}
		files, _ := mapTrackFiles(lf.Path, d.files, tracks, ns)
		adopted := 0
		err = s.Store.Write(ctx, func(tx *sql.Tx) error {
			for _, f := range files {
				if f.slot == 0 {
					a.Unknown = append(a.Unknown, f.Path+": "+f.Why)
					continue
				}
				var exists int
				if err := tx.QueryRow("SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ? AND state != 'gone'", v.ID, f.Path).Scan(&exists); err != nil {
					return err
				}
				if exists > 0 {
					continue
				}
				ext := strings.ToLower(path.Ext(f.Path))
				release := d.name + " [" + adoptedFormat(filepath.Join(lf.Path, filepath.FromSlash(f.Path)), ext) + "]"
				file := &mediafiles.File{SlotIDs: []int64{f.slot}, VersionID: v.ID, RelativePath: f.Path, Size: f.Size, AddedAt: s.now(),
					Quality: mediafiles.Quality{Source: strings.TrimPrefix(ext, ".")}, Languages: []string{}, MediaInfo: map[string]any{},
					ReleaseName: &release, Origin: "adopted", PlacedBy: "none", Role: "main", State: "present"}
				fileID, err := database.Insert(tx, "files", file, by)
				if err != nil {
					return err
				}
				ge := mediafiles.ReachesGoodEnough(release, pr.Body)
				state, why := mediafiles.SlotInLibrary, "adopted from "+a.FolderName
				if !ge && v.KeepUpToDate && t.KeepUpToDate {
					state, why = mediafiles.SlotBetterWanted, "adopted below good enough, so a better copy is sought"
				}
				if _, err := tx.Exec(`UPDATE slots SET file_id = ?, good_enough = ?, state = ?, last_reason = ?, held_release_id = NULL WHERE id = ? AND state != 'removed'`,
					fileID, ge, string(state), why, f.slot); err != nil {
					return err
				}
				adopted++
			}
			return nil
		})
		if err != nil {
			return err
		}
		a.Adopted += adopted
		if adopted > 0 {
			a.Albums++
		}
	}
	return nil
}

// recordFoundTracks records unknown audio files in an artist's folder as
// found Files: shown, never renamed, moved or deleted, filling no Slot.
func (s *Service) recordFoundTracks(ctx context.Context, lf mediafiles.LibraryFolder, v mediafiles.Version, known map[string]bool, by domain.Actor) (int, error) {
	db := s.Store.Read()
	n := 0
	for _, f := range listAudio(lf.Path, v.FolderName, maxArtistFiles) {
		if known[strings.ToLower(f.Path)] {
			continue
		}
		var exists int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", v.ID, f.Path).Scan(&exists)
		if exists > 0 {
			continue
		}
		release := strings.TrimSuffix(path.Base(f.Path), path.Ext(f.Path))
		file := &mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: f.Path, Size: f.Size, AddedAt: s.now(), Languages: []string{},
			MediaInfo: map[string]any{}, ReleaseName: &release, Origin: "found", PlacedBy: "none", Role: "main", State: "present"}
		if err := s.Store.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", file, by); return err }); err != nil {
			return n, err
		}
		n++
		known[strings.ToLower(f.Path)] = true
	}
	return n, nil
}

// adoptedFormat is the format word an adopted file's Release name gets: its
// extension, or for a file that may hold either kind (.m4a, .mka, .mp4)
// the codec read from its headers, so an ALAC library is known as lossless.
func adoptedFormat(abs, ext string) string {
	word := strings.ToUpper(strings.TrimPrefix(ext, "."))
	switch ext {
	case ".m4a", ".mka", ".mp4":
		if info, err := mediafiles.Inspect(abs); err == nil && info.AudioWord() != "" {
			return info.AudioWord()
		}
	}
	return word
}
