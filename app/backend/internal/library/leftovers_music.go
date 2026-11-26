package library

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// maxMusicLeftovers caps how many of an artist's unmatched files are read
// again for their tags when the list is asked for.
const maxMusicLeftovers = 2000

// MusicLeftover is where an unmatched music file came closest: the album and
// track it nearly fits (0 when none), or the other artist its tags name.
type MusicLeftover struct {
	AlbumID   int64  `json:"album_id,omitempty"`
	AlbumName string `json:"album_name,omitempty"`
	TrackID   int64  `json:"track_id,omitempty"`
	Owner     string `json:"owner,omitempty"`
}

// PressingHint says the unmatched files of an album fit another of its
// Pressings better than the chosen one. It is only offered: the person
// switches.
type PressingHint struct {
	AlbumID      int64  `json:"album_id"`
	AlbumName    string `json:"album_name"`
	PressingID   int64  `json:"pressing_id"`
	PressingName string `json:"pressing_name"`
	TrackCount   int    `json:"track_count"`
	Fits         int    `json:"fits"`   // files that fit a track of that Pressing
	Chosen       int    `json:"chosen"` // files that fit a track of the chosen one
	Files        int    `json:"files"`  // unmatched files of the album
}

// LeftoverList is Leftovers with, for an artist, the Pressings its unmatched files fit better.
func (s *Service) LeftoverList(ctx context.Context, titleID int64) ([]LeftoverView, []PressingHint, error) {
	ls, err := s.Leftovers(ctx, titleID)
	if err != nil {
		return nil, nil, err
	}
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil || t.Kind != "artist" {
		return ls, []PressingHint{}, err
	}
	hints, err := s.musicLeftovers(ctx, t, ls)
	return ls, hints, err
}

// musicLeftovers words an artist's leftovers: the reason (read again now, so
// it is current), the album and track each came closest to, and the
// Pressings that fit them better. One artist is read at a time.
func (s *Service) musicLeftovers(ctx context.Context, t mediafiles.Title, ls []LeftoverView) ([]PressingHint, error) {
	db := s.Store.Read()
	at := map[int64]int{}
	for i := range ls {
		at[ls[i].FileID] = i
		if ls[i].Reason == "episode_not_listed" {
			ls[i].Reason = "music_not_listed"
		}
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY id", t.ID)
	if err != nil {
		return nil, err
	}
	byAlbum := map[int64][]importer.FoundMusicFile{}
	read := 0
	var owners *importer.MusicOwners
	for _, v := range vs {
		if read >= maxMusicLeftovers {
			break
		}
		found, err := database.Select[mediafiles.File](ctx, db, "files",
			"version_id = ? AND origin = 'found' AND state = 'present' AND role = 'main' AND slot_ids = '[]' ORDER BY size DESC LIMIT ?", v.ID, maxMusicLeftovers-read)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			continue
		}
		read += len(found)
		var lf mediafiles.LibraryFolder
		if err := database.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
			return nil, err
		}
		files, _, err := foundMusicFiles(ctx, db, lf, v, t, found)
		if err != nil {
			return nil, err
		}
		if err := importer.MatchFoundMusic(ctx, db, t, v.ID, ns.MusicReadTags, files); err != nil {
			return nil, err
		}
		for k, ff := range files {
			li, ok := at[found[k].ID]
			if !ok {
				continue
			}
			l := &ls[li]
			if ff.SlotID != 0 {
				ff.Reason = importer.FoundUnsure // it fits now; the next scan links it
			}
			if stored := l.Reason; strings.HasPrefix(stored, "music_") || stored == "" {
				l.Reason = ff.Reason
			}
			if l.Reason == importer.FoundOtherArtist || ff.Reason == importer.FoundNoAlbum && !ff.Tags.Empty() {
				if owners == nil {
					if owners, err = importer.NewMusicOwners(ctx, db); err != nil {
						return nil, err
					}
				}
				to, err := owners.Owner(ctx, t.ID, ff.Tags, ff.AlbumFolder, ff.AlbumYear)
				if err != nil {
					return nil, err
				}
				if to != 0 {
					var name string
					_ = db.QueryRowContext(ctx, "SELECT name FROM titles WHERE id = ?", to).Scan(&name)
					l.Reason, l.Music = importer.FoundOtherArtist, &MusicLeftover{Owner: name}
					continue
				}
				if l.Reason == importer.FoundOtherArtist {
					l.Reason = importer.FoundNoAlbum
				}
			}
			if ff.AlbumID != 0 {
				l.Music = &MusicLeftover{AlbumID: ff.AlbumID, TrackID: ff.ItemID}
				if ff.Reason == importer.FoundNoTrack || ff.Reason == importer.FoundUnsure {
					byAlbum[ff.AlbumID] = append(byAlbum[ff.AlbumID], ff)
				}
			}
		}
	}
	// Album names, and the Pressings that fit an album's unmatched files better.
	names := map[int64]string{}
	nameOf := func(id int64) string {
		if n, ok := names[id]; ok {
			return n
		}
		var n string
		_ = db.QueryRowContext(ctx, "SELECT name FROM parts WHERE id = ?", id).Scan(&n)
		names[id] = n
		return n
	}
	for i := range ls {
		if m := ls[i].Music; m != nil && m.AlbumID != 0 {
			m.AlbumName = nameOf(m.AlbumID)
		}
	}
	hints := []PressingHint{}
	for album, files := range byAlbum {
		ps, err := database.Select[domain.AlbumPressing](ctx, db, "pressings", "part_id = ? AND dropped_at IS NULL ORDER BY id", album)
		if err != nil {
			return nil, err
		}
		if h, ok := pressingHint(files, ps); ok {
			h.AlbumID, h.AlbumName = album, nameOf(album)
			hints = append(hints, h)
		}
	}
	sort.Slice(hints, func(i, j int) bool { return hints[i].AlbumName < hints[j].AlbumName })
	return hints, nil
}

var leadNumber = regexp.MustCompile(`^\s*(?:\d{1,3}[\s.\-_]+)+`)

// pressingHint scores each Pressing by how many files fit one of its tracks
// (by recording id, pressing id, title, or the file name when untagged). The
// best one is offered when it is not the chosen one, fits at least two
// files and half of them, and fits more than the chosen one.
func pressingHint(files []importer.FoundMusicFile, ps []domain.AlbumPressing) (PressingHint, bool) {
	if len(files) < 2 || len(ps) < 2 {
		return PressingHint{}, false
	}
	fits := func(p domain.AlbumPressing) int {
		n := 0
		for _, f := range files {
			if f.Tags.ReleaseID != "" && strings.EqualFold(f.Tags.ReleaseID, p.MBID) {
				n++
				continue
			}
			title := f.Tags.Title
			if title == "" {
				base := path.Base(strings.ReplaceAll(f.Path, "\\", "/"))
				title = leadNumber.ReplaceAllString(strings.TrimSuffix(base, path.Ext(base)), "")
			}
			for _, tr := range p.Tracks {
				if f.Tags.RecordingID != "" && strings.EqualFold(f.Tags.RecordingID, tr.RecordingID) || title != "" && importer.SameSong(title, tr.Name) {
					n++
					break
				}
			}
		}
		return n
	}
	chosen, bestFits := 0, 0
	var best *domain.AlbumPressing
	for i := range ps {
		n := fits(ps[i])
		if ps[i].Chosen {
			chosen = n
			continue
		}
		if n > bestFits {
			best, bestFits = &ps[i], n
		}
	}
	if best == nil || bestFits < 2 || bestFits*2 < len(files) || bestFits <= chosen {
		return PressingHint{}, false
	}
	name := best.Name
	if best.Disambiguation != "" {
		name += " (" + best.Disambiguation + ")"
	}
	return PressingHint{PressingID: best.ID, PressingName: name, TrackCount: best.TrackCount, Fits: bestFits, Chosen: chosen, Files: len(files)}, true
}

// UnmatchedArtist is one artist with found music files that fit no track.
type UnmatchedArtist struct {
	TitleID int64  `json:"title_id"`
	Name    string `json:"name"`
	Files   int    `json:"files"`
}

// UnmatchedMusic counts the found music files the last scans could not
// match, left alone ones aside: the total and the artists with the most.
func (s *Service) UnmatchedMusic(ctx context.Context) (int, []UnmatchedArtist, error) {
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT t.id, t.name, count(*) FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id
		WHERE t.kind = 'artist' AND t.state != 'removed' AND f.origin = 'found' AND f.state = 'present' AND f.role = 'main' AND f.leftover_dismissed = 0
		AND substr(f.leftover_reason, 1, 6) = 'music_' GROUP BY t.id ORDER BY count(*) DESC, t.name`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	total, out := 0, []UnmatchedArtist{}
	for rows.Next() {
		var a UnmatchedArtist
		if err := rows.Scan(&a.TitleID, &a.Name, &a.Files); err != nil {
			return 0, nil, err
		}
		total += a.Files
		if len(out) < 50 {
			out = append(out, a)
		}
	}
	return total, out, rows.Err()
}
