package importer

import (
	"context"
	"database/sql"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/tags"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Why a found music file fills no track; kept on the file so the artist page
// can list it. Every key starts with "music_", so a later scan tries it again.
const (
	FoundNoAlbum     = "music_no_album"     // no album of the artist fits its tags or folder
	FoundTwoAlbum    = "music_two_album"    // more than one album fits and nothing tells them apart
	FoundAlbumOff    = "music_album_off"    // the album is left out by the artist's album kinds
	FoundNoTrack     = "music_no_track"     // the album has no track its number, title or file name points at
	FoundUnsure      = "music_unsure"       // a track fits, but its number, title or length disagree
	FoundHasFile     = "music_has_file"     // the track already has a file
	FoundDuplicate   = "music_duplicate"    // a bigger copy of the same track was linked
	FoundOtherArtist = "music_other_artist" // its tags name another artist of the Library, in another Library folder
)

// FoundMusicFile is an audio file found in an artist's folder that fills no
// track yet. FolderPartID is the album its album folder was matched to by
// name (0 when none fits well); SlotID is set to the track it fills. When it
// fills none, Reason says why, and AlbumID and ItemID are the album and track
// it came closest to (0 when none).
type FoundMusicFile struct {
	Path         string // absolute
	Size         int64
	AlbumFolder  string // the album folder name with the artist and year cut off
	AlbumYear    int
	FolderPartID int64
	SlotID       int64

	Reason  string
	AlbumID int64
	ItemID  int64
	Tags    tags.Tags
}

// MatchFoundMusic matches the found files of one artist (biggest first) to
// the empty Slots of versionID: the album by the file's MusicBrainz ids,
// then its album tag, then its album folder; the track by its recording id,
// then its disc and track number, then its file name, with the play length
// as a second opinion. A file is matched only when the album is sure and the
// track number and name agree, each track takes one file, and a track that
// already has a file takes none. Only this artist is read.
func MatchFoundMusic(ctx context.Context, db *sql.DB, t mediafiles.Title, versionID int64, readTags bool, files []FoundMusicFile) error {
	a, err := loadMusicArtist(ctx, db, t, versionID)
	if err != nil {
		return err
	}
	byPart := map[int64]*musicAlbum{}
	for _, al := range a.albums {
		byPart[al.part.ID] = al
	}
	taken := map[int64]bool{}
	for i := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := &files[i]
		f.SlotID, f.Reason, f.AlbumID, f.ItemID = 0, "", 0, 0
		if readTags {
			f.Tags, _ = tags.ReadFile(f.Path)
		}
		al, why, loose := a.album(f.Tags, f.AlbumFolder, f.AlbumYear)
		if al == nil || loose {
			if byFolder := byPart[f.FolderPartID]; byFolder != nil || al == nil {
				al, loose = byFolder, false
			}
		}
		if al == nil {
			f.Reason = FoundNoAlbum
			if why == MusicTwoAlbum {
				f.Reason = FoundTwoAlbum
			}
			continue
		}
		f.AlbumID = al.part.ID
		if !al.part.Included {
			f.Reason = FoundAlbumOff
			continue
		}
		tr, sure := al.track(&ByHandFile{Path: f.Path, Size: f.Size}, f.Tags)
		if tr == nil {
			f.Reason = FoundNoTrack
			continue
		}
		f.ItemID = tr.item.ID
		switch {
		case loose || !sure || tr.slot == 0:
			f.Reason = FoundUnsure
		case tr.filled:
			f.Reason = FoundHasFile
		case taken[tr.slot]:
			f.Reason = FoundDuplicate
		default:
			taken[tr.slot], f.SlotID = true, tr.slot
		}
	}
	return nil
}

// MusicOwners finds the artist a found file belongs to; one is made per
// scan and reads each artist's albums at most once.
type MusicOwners struct {
	db  *sql.DB
	lib *musicLibrary
}

// NewMusicOwners reads the Library's artists.
func NewMusicOwners(ctx context.Context, db *sql.DB) (*MusicOwners, error) {
	artists, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed'")
	if err != nil {
		return nil, err
	}
	return &MusicOwners{db: db, lib: &musicLibrary{db: db, artists: artists, loaded: map[int64]*musicArtist{}}}, nil
}

// Owner is the other artist of the Library a found file belongs to,
// by its tags: the artist with an album of the file's release group or
// pressing, else an artist its album artist or track artist names, when that
// artist has an album the file surely fits. A "Various Artists" compilation
// goes to the artist whose album list holds the compilation. 0 when none, or
// when it is self.
func (o *MusicOwners) Owner(ctx context.Context, self int64, tg tags.Tags, albumFolder string, albumYear int) (int64, error) {
	if tg.Empty() {
		return 0, nil
	}
	var cands []int64
	add := func(id int64) {
		for _, c := range cands {
			if c == id {
				return
			}
		}
		cands = append(cands, id)
	}
	if tg.ReleaseGroupID != "" {
		rows, err := o.db.QueryContext(ctx, `SELECT DISTINCT p.title_id FROM parts p JOIN titles t ON t.id = p.title_id
			WHERE p.kind = 'album' AND p.dropped_at IS NULL AND t.state != 'removed' AND lower(json_extract(p.source_ids, '$.musicbrainz')) = ?`, strings.ToLower(tg.ReleaseGroupID))
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				add(id)
			}
		}
		_ = rows.Close()
	}
	lib := o.lib
	for _, n := range []string{tg.AlbumArtist, mainArtist(tg.AlbumArtist), tg.Artist, mainArtist(tg.Artist)} {
		if hit := lib.artistNamed(n); hit != nil {
			add(hit.ID)
		}
	}
	for _, id := range cands {
		if id == self {
			continue
		}
		var ti *mediafiles.Title
		for i := range lib.artists {
			if lib.artists[i].ID == id {
				ti = &lib.artists[i]
			}
		}
		if ti == nil {
			continue
		}
		a, err := lib.artist(ctx, *ti)
		if err != nil {
			return 0, err
		}
		if al, _, loose := a.album(tg, albumFolder, albumYear); al != nil && !loose {
			return id, nil
		}
	}
	return 0, nil
}
