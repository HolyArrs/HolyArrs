package importer

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/tags"
)

// Why a music file found by hand has no match; the page words each one.
const (
	MusicNoArtist = "no_artist" // no artist in the Library has the name the file gives
	MusicNoAlbum  = "no_album"  // the artist has no album with the name the file gives
	MusicTwoAlbum = "two_album" // more than one of the artist's albums has that name
	MusicNoTrack  = "no_track"  // the album has no track the file's number or name points at
	MusicNoFacts  = "no_facts"  // the file has no tags and its folders name no artist
)

// musicArtist is one artist of the Library with its albums and tracks, read once per scan.
type musicArtist struct {
	t      mediafiles.Title
	main   int64
	albums []*musicAlbum
}

type musicAlbum struct {
	part     mediafiles.Part
	year     int
	releases map[string]bool // the MusicBrainz ids of its pressings
	tracks   []musicTrack
}

type musicTrack struct {
	item   mediafiles.Item
	disc   int
	number int
	name   string
	slot   int64
	filled bool
}

// musicLibrary is what the by-hand scan matches music files against: the
// artists already in the Library, their albums and their tracks.
type musicLibrary struct {
	db       *sql.DB
	readTags bool
	artists  []mediafiles.Title
	loaded   map[int64]*musicArtist
}

func newMusicLibrary(ctx context.Context, db *sql.DB) (*musicLibrary, error) {
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	artists, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed'")
	if err != nil {
		return nil, err
	}
	return &musicLibrary{db: db, readTags: ns.MusicReadTags, artists: artists, loaded: map[int64]*musicArtist{}}, nil
}

func (ml *musicLibrary) artist(ctx context.Context, t mediafiles.Title) (*musicArtist, error) {
	if a := ml.loaded[t.ID]; a != nil {
		return a, nil
	}
	a := &musicArtist{t: t}
	vers, err := mediafiles.VersionsOfTitle(ctx, ml.db, t.ID)
	if err != nil {
		return nil, err
	}
	if len(vers) > 0 {
		a.main = vers[0].ID
	}
	parts, err := database.Select[mediafiles.Part](ctx, ml.db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL", t.ID)
	if err != nil {
		return nil, err
	}
	items, err := database.Select[mediafiles.Item](ctx, ml.db, "items", "title_id = ? AND kind = 'track' AND dropped_at IS NULL", t.ID)
	if err != nil {
		return nil, err
	}
	slots, err := database.Select[mediafiles.Slot](ctx, ml.db, "slots", "version_id = ?", a.main)
	if err != nil {
		return nil, err
	}
	slotOf := map[int64]mediafiles.Slot{}
	for _, s := range slots {
		slotOf[s.ItemID] = s
	}
	byPart := map[int64]*musicAlbum{}
	for _, p := range parts {
		al := &musicAlbum{part: p, releases: map[string]bool{}}
		if p.ReleaseDate != nil {
			al.year = p.ReleaseDate.Year()
		}
		for _, pr := range p.Pressings {
			if pr.SourceID != "" {
				al.releases[strings.ToLower(pr.SourceID)] = true
			}
		}
		byPart[p.ID] = al
		a.albums = append(a.albums, al)
	}
	for _, it := range items {
		if it.PartID == nil || byPart[*it.PartID] == nil {
			continue
		}
		tr := musicTrack{item: it, disc: 1}
		if it.Numbers.Disc != nil && *it.Numbers.Disc > 0 {
			tr.disc = *it.Numbers.Disc
		}
		if it.Numbers.Track != nil {
			tr.number = *it.Numbers.Track
		}
		if it.Name != nil {
			tr.name = *it.Name
		}
		if s, ok := slotOf[it.ID]; ok {
			tr.slot, tr.filled = s.ID, s.FileID != nil
		}
		al := byPart[*it.PartID]
		al.tracks = append(al.tracks, tr)
	}
	ml.loaded[t.ID] = a
	return a, nil
}

var yearTail = regexp.MustCompile(`\s*[\(\[]\s*((?:19|20)\d{2})\s*[\)\]]\s*$|\s*[\(\[][^\)\]]*[\)\]]\s*$`)

// folderAlbum strips a trailing "(2001)" or "[FLAC]" off an album folder's name
// and gives the year when one was there.
func folderAlbum(name string) (string, int) {
	year := 0
	for i := 0; i < 3; i++ {
		m := yearTail.FindStringSubmatchIndex(name)
		if m == nil {
			break
		}
		if m[2] >= 0 && year == 0 {
			year, _ = strconv.Atoi(name[m[2]:m[3]])
		}
		name = strings.TrimSpace(name[:m[0]])
	}
	return strings.TrimSpace(name), year
}

var tagTail = regexp.MustCompile(`(?i)\s*[\(\[]\s*(?:(?:19|20)\d{2}|flac|mp3|aac|alac|ogg|opus|wav|320|v0|cd|web|hi-?res|\d{2}-?bit)\s*[\)\]]\s*$`)

// tagAlbum strips only a trailing year or file format off an album tag.
func tagAlbum(name string) string {
	for i := 0; i < 3; i++ {
		name = tagTail.ReplaceAllString(name, "")
	}
	return strings.TrimSpace(name)
}

// sameName compares two names leaving out case, marks and a leading "The".
func sameName(a, b string) bool {
	fa, fb := fold(a), fold(b)
	if fa == "" || fb == "" {
		return false
	}
	if fa == fb {
		return true
	}
	cut := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.TrimPrefix(s, "the ")
		return fold(strings.TrimSuffix(s, ", the"))
	}
	return cut(a) != "" && cut(a) == cut(b)
}

// musicFolders reads the artist and album folder names out of a file's path:
// ".../Artist/Album (2001)/CD1/01 Song.flac" or ".../Artist - Album/01 Song.flac".
func musicFolders(p string) (artist, album string, year int) {
	dirs := strings.Split(filepath.ToSlash(filepath.Dir(p)), "/")
	if n := len(dirs); n > 0 && discDir.MatchString(dirs[n-1]) {
		dirs = dirs[:n-1]
	}
	if len(dirs) == 0 {
		return "", "", 0
	}
	album = dirs[len(dirs)-1]
	_, year = folderAlbum(album)
	if len(dirs) > 1 {
		artist = dirs[len(dirs)-2]
	}
	return artist, album, year
}

var featTail = regexp.MustCompile(`(?i)\s*[\(\[]?\s*\b(?:feat\.?|ft\.|featuring)\s.*$`)

// mainArtist cuts a "feat. Someone" tail off an artist tag; "" when there is none.
func mainArtist(name string) string {
	if cut := strings.TrimSpace(featTail.ReplaceAllString(name, "")); cut != name {
		return cut
	}
	return ""
}

func (ml *musicLibrary) artistNamed(names ...string) *mediafiles.Title {
	for _, n := range names {
		if n == "" {
			continue
		}
		var hit *mediafiles.Title
		for i := range ml.artists {
			t := &ml.artists[i]
			ok := sameName(t.Name, n)
			for _, o := range OtherNames(*t) {
				ok = ok || sameName(o, n)
			}
			if ok {
				if hit != nil && hit.ID != t.ID {
					hit = nil
					break
				}
				hit = t
			}
		}
		if hit != nil {
			return hit
		}
	}
	return nil
}

// musicGuess is how sure each step of a music match is.
type musicGuess struct {
	byTags bool // the artist and album came from the file's own tags
}

// guessMusicByHand matches one audio file to an artist, album and track of the Library.
func (ml *musicLibrary) guessMusicByHand(ctx context.Context, f *ByHandFile) error {
	var tg tags.Tags
	if ml.readTags {
		tg, _ = tags.ReadFile(f.Path)
	}
	dirArtist, dirAlbum, dirYear := musicFolders(f.Path)
	if f.InLibrary {
		f.Note = "already a file in the Library"
	}
	// The artist: its MusicBrainz id, then the tags, then the folders.
	var ti *mediafiles.Title
	byTags := false
	for _, id := range strings.Fields(tg.ArtistID) {
		for i := range ml.artists {
			if ti == nil && strings.EqualFold(ml.artists[i].SourceIDs["musicbrainz"], id) {
				ti, byTags = &ml.artists[i], true
			}
		}
	}
	if ti == nil {
		if ti = ml.artistNamed(tg.AlbumArtist, tg.Artist, mainArtist(tg.AlbumArtist), mainArtist(tg.Artist)); ti != nil {
			byTags = true
		}
	}
	if ti == nil {
		ti = ml.artistNamed(dirArtist)
	}
	if ti == nil && dirAlbum != "" {
		// "Artist - Album" in one folder.
		if left, right, ok := strings.Cut(dirAlbum, " - "); ok {
			if ti = ml.artistNamed(left); ti != nil {
				dirAlbum = right
			}
		}
	}
	if ti == nil {
		name := tg.AlbumArtist
		if name == "" {
			name = tg.Artist
		}
		if name == "" {
			name = dirArtist
		}
		f.Guess.Reason, f.Guess.Detail = MusicNoArtist, name
		if tg.Empty() && dirArtist == "" {
			f.Guess.Reason = MusicNoFacts
		}
		return nil
	}
	a, err := ml.artist(ctx, *ti)
	if err != nil {
		return err
	}
	f.Guess.TitleID, f.Guess.TitleName, f.Guess.TitleKind, f.Guess.VersionID = ti.ID, ti.Name, "artist", a.main
	al, reason, loose := a.album(tg, dirAlbum, dirYear)
	if al == nil {
		f.Guess.Reason, f.Guess.Detail = reason, tg.Album
		if f.Guess.Detail == "" {
			f.Guess.Detail = dirAlbum
		}
		return nil
	}
	f.Guess.AlbumID, f.Guess.AlbumName, f.Guess.AlbumYear = al.part.ID, al.part.Name, al.year
	tr, sure := al.track(f, tg)
	if tr == nil {
		f.Guess.Reason = MusicNoTrack
		return nil
	}
	f.Guess.Disc, f.Guess.Track, f.Guess.TrackName = tr.disc, tr.number, tr.name
	f.Guess.SlotID, f.Guess.HasFile = tr.slot, tr.filled
	switch {
	case loose:
		f.Guess.Confidence = 0.4
	case byTags && sure:
		f.Guess.Confidence = 1
	case sure:
		f.Guess.Confidence = 0.7
	default:
		f.Guess.Confidence = 0.4
	}
	return nil
}

// album finds the album by its MusicBrainz ids, then by the album tag, then by the folder name.
func (a *musicArtist) album(tg tags.Tags, dirAlbum string, dirYear int) (*musicAlbum, string, bool) {
	for _, al := range a.albums {
		if tg.ReleaseGroupID != "" && strings.EqualFold(al.part.SourceIDs["musicbrainz"], tg.ReleaseGroupID) || tg.ReleaseID != "" && al.releases[tg.ReleaseID] {
			return al, "", false
		}
	}
	reason := MusicNoAlbum
	for _, name := range []string{tg.Album, dirAlbum} {
		stripped, year := folderAlbum(name)
		if year == 0 {
			year = dirYear
		}
		// An album named exactly as the tag or folder wins over one found after cutting the tail.
		var hits, cut []*musicAlbum
		exact := tagAlbum(name)
		for _, al := range a.albums {
			if sameName(al.part.Name, name) || sameName(al.part.Name, exact) {
				hits = append(hits, al)
			} else if sameName(al.part.Name, stripped) {
				cut = append(cut, al)
			}
		}
		if len(hits) == 0 {
			hits = cut
		}
		// A same-name single with no tracks listed can take no file, so it never blocks the album.
		if len(hits) > 1 {
			var withTracks []*musicAlbum
			for _, al := range hits {
				if len(al.tracks) > 0 {
					withTracks = append(withTracks, al)
				}
			}
			if len(withTracks) > 0 {
				hits = withTracks
			}
		}
		if len(hits) > 1 && year > 0 {
			var same []*musicAlbum
			for _, al := range hits {
				if al.year == year {
					same = append(same, al)
				}
			}
			hits = same
		}
		switch {
		case len(hits) == 1:
			// A name that only matches once a "(Live)" or "(Deluxe Edition)" is cut off may be another recording.
			// A folder hit after an album tag that found nothing is a guess too.
			loose := !sameName(hits[0].part.Name, name) && !sameName(hits[0].part.Name, exact) || name == dirAlbum && tg.Album != "" && !sameName(tg.Album, dirAlbum)
			return hits[0], "", loose
		case len(hits) > 1:
			reason = MusicTwoAlbum
		}
	}
	return nil, reason, false
}

// track finds the track by its MusicBrainz recording id, then by the tag's
// disc and track number, then by the file name; sure is false when the number
// and the name disagree.
func (al *musicAlbum) track(f *ByHandFile, tg tags.Tags) (*musicTrack, bool) {
	discs := 1
	for _, tr := range al.tracks {
		discs = max(discs, tr.disc)
	}
	if tg.RecordingID != "" {
		for i := range al.tracks {
			if rid := al.tracks[i].item.RecordingID; rid != nil && strings.EqualFold(*rid, tg.RecordingID) {
				return &al.tracks[i], true
			}
		}
	}
	if tg.Track > 0 {
		disc := tg.Disc
		if disc == 0 {
			disc = discOf(f.Path)
		}
		var hit *musicTrack
		for i := range al.tracks {
			tr := &al.tracks[i]
			if tr.number == tg.Track && (tr.disc == disc || disc == 0 && discs == 1) {
				hit = tr
				break
			}
		}
		if hit != nil {
			return hit, tg.Title == "" || fold(tg.Title) == fold(hit.name)
		}
	}
	// No usable tags: the file name, the way an album download is matched.
	ts := make([]Track, len(al.tracks))
	for i, tr := range al.tracks {
		ts[i] = Track{ItemID: tr.item.ID, Disc: tr.disc, Number: tr.number, Name: tr.name, SlotID: int64(i + 1), Wanted: true}
	}
	steps := PlanTracks([]Candidate{{Path: f.Path, Size: f.Size}}, AlbumTarget{Tracks: ts})
	if len(steps) == 1 && steps[0].Method == "hardlink" {
		return &al.tracks[steps[0].TargetSlotIDs[0]-1], true
	}
	return nil, false
}

// trackTarget is where a track file chosen by hand goes: the same naming an
// album download uses.
func trackTarget(ctx context.Context, db database.Querier, ns mediafiles.NamingSettings, lf mediafiles.LibraryFolder, ver mediafiles.Version,
	ti mediafiles.Title, item mediafiles.Item, src string) (string, error) {
	if item.PartID == nil {
		return "", errors.New("the track belongs to no album")
	}
	at, err := albumTarget(ctx, db, lf, ver, ti, []mediafiles.Item{item}, map[int64]int64{}, nil, true, ns)
	if err != nil {
		return "", err
	}
	_, discs, err := AlbumFacts(ctx, db, ti.Name, *item.PartID)
	if err != nil {
		return "", err
	}
	return filepath.Join(lf.Path, filepath.FromSlash(at.trackFile(at.Tracks[0], strings.ToLower(filepath.Ext(src)), discs, src))), nil
}
