package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// ErrBadAlbumFolder says the folder named for an album is not one folder of
// the artist's folder.
var ErrBadAlbumFolder = errors.New("send the name of one album folder inside the artist's folder")

// AlbumFolderLink says what linking an album folder by hand did.
type AlbumFolderLink struct {
	Folder string `json:"folder"`
	// Files is the audio files the folder holds, Linked the ones that became
	// the file of a track.
	Files   int      `json:"files"`
	Linked  int      `json:"linked"`
	Unknown []string `json:"unknown,omitempty"` // the files left alone, and why
}

// LinkAlbumFolder links one album folder of an artist's folder to the album's
// tracks by hand, for the folders a scan cannot place: the folder's name may
// name no album of the artist, or a character a file system cannot hold may
// have replaced one of its own ("5_55" for "5:55"). Each file that holds a
// track becomes the file of that track; the ones that hold none are left
// alone and say why. Nothing on disk is renamed, moved or written.
//
// This is the album's own "force the association", as a show or a movie has.
func (s *Service) LinkAlbumFolder(ctx context.Context, partID int64, folder string, by domain.Actor) (AlbumFolderLink, error) {
	out := AlbumFolderLink{Folder: folder, Unknown: []string{}}
	if !oneFolderName(folder) {
		return out, ErrBadAlbumFolder
	}
	db := s.Store.Read()
	var p mediafiles.Part
	if err := database.Get(ctx, db, "parts", partID, &p); err != nil || p.Kind != "album" || p.DroppedAt != nil {
		return out, ErrNotAlbum
	}
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", p.TitleID, &t); err != nil {
		return out, err
	}
	if t.Kind != "artist" {
		return out, ErrNotArtist
	}
	vid, err := mediafiles.MainVersionID(ctx, db, t.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrNotArtist
		}
		return out, err
	}
	var v mediafiles.Version
	if err := database.Get(ctx, db, "versions", vid, &v); err != nil {
		return out, err
	}
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
		return out, ErrNoFolder
	}
	if lf.Kind != "music" {
		return out, ErrNotMusicFolder
	}
	rel := folder
	if strings.TrimSpace(v.FolderName) != "" {
		rel = path.Join(v.FolderName, folder)
	}
	if fi, err := os.Lstat(filepath.Join(lf.Path, filepath.FromSlash(rel))); err != nil || !fi.IsDir() {
		return out, ErrBadAlbumFolder
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return out, err
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, db, "pick_rules", v.PickRulesID, &pr); err != nil {
		return out, err
	}
	tracks, err := albumFolderTracks(ctx, db, p.ID, v.ID)
	if err != nil {
		return out, err
	}
	if len(tracks) == 0 {
		return out, ErrNotAlbum
	}
	files := listAudio(lf.Path, rel, maxArtistFiles)
	out.Files = len(files)
	if len(files) == 0 {
		return out, nil
	}
	mapped, _ := mapTrackFiles(lf.Path, files, tracks, ns)
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, f := range mapped {
			if f.slot == 0 {
				out.Unknown = append(out.Unknown, f.Path+": "+f.Why)
				continue
			}
			free, err := mediafiles.SlotStillEmptyTx(ctx, tx, f.slot, v.ID)
			if err != nil {
				return err
			}
			if !free {
				out.Unknown = append(out.Unknown, f.Path+": the track already has a file")
				continue
			}
			id, err := folderFileRow(ctx, tx, v.ID, f.Path, f.Size, s.now(), by)
			if err != nil {
				return err
			}
			release := path.Base(path.Dir(f.Path)) + " [" + adoptedFormat(filepath.Join(lf.Path, filepath.FromSlash(f.Path)), strings.ToLower(path.Ext(f.Path))) + "]"
			if err := mediafiles.SetReleaseNameTx(ctx, tx, id, release); err != nil {
				return err
			}
			ge := mediafiles.ReachesGoodEnough(release, pr.Body)
			state, why := mediafiles.SlotInLibrary, "linked to its album folder by hand"
			if !ge && v.KeepUpToDate && t.KeepUpToDate {
				state, why = mediafiles.SlotBetterWanted, "linked by hand below good enough, so a better copy is sought"
			}
			if err := mediafiles.LinkFileInPlaceTx(ctx, tx, id, []int64{f.slot}, ge, state, why, HoldingSlots, s.now(), by); err != nil {
				return err
			}
			out.Linked++
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if out.Linked > 0 {
		id := t.ID
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &id,
			Summary:  fmt.Sprintf("%s: %d files of %s linked to the album by hand", t.Name, out.Linked, folder),
			Data:     map[string]any{"library_folder_id": lf.ID, "folder": rel, "album": p.Name, "linked": out.Linked},
			GroupKey: GroupKey(id)})
	}
	return out, nil
}

// oneFolderName says whether s names one folder: not empty, not "." or "..",
// no separator, and not a folder that is hidden or HolyArrs' own.
func oneFolderName(s string) bool {
	if s == "" || s == "." || s == ".." || !filepath.IsLocal(s) || strings.ContainsAny(s, `/\`) || strings.HasPrefix(s, ".") {
		return false
	}
	return s != importer.HoldingDir && !NasSystemDir(s)
}

// albumFolderTracks are the album's tracks with the Slots of one Version, in
// the shape the album import matches files with.
func albumFolderTracks(ctx context.Context, db Reader, partID, versionID int64) ([]importer.Track, error) {
	items, err := database.Select[mediafiles.Item](ctx, db, "items", "part_id = ? AND kind = 'track' AND dropped_at IS NULL ORDER BY id", partID)
	if err != nil {
		return nil, err
	}
	tracks := make([]importer.Track, 0, len(items))
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
		slotID, err := mediafiles.SlotIDOfItem(ctx, db, it.ID, versionID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue // no Slot of this Version holds the track: it takes no file
			}
			return nil, err
		}
		tr.SlotID = slotID
		tracks = append(tracks, tr)
	}
	return tracks, nil
}

// folderFileRow is the File row of a file found in a Library folder: the one
// a scan already recorded, or a new adopted one.
func folderFileRow(ctx context.Context, tx *sql.Tx, versionID int64, rel string, size int64, at time.Time, by domain.Actor) (int64, error) {
	id, err := mediafiles.FileIDOnPath(ctx, tx, versionID, rel)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	release := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	f := &mediafiles.File{SlotIDs: []int64{}, VersionID: versionID, RelativePath: rel, Size: size, AddedAt: at,
		Quality: mediafiles.Quality{Source: strings.TrimPrefix(strings.ToLower(path.Ext(rel)), ".")}, Languages: []string{}, MediaInfo: map[string]any{},
		ReleaseName: &release, Origin: "adopted", PlacedBy: "none", Role: "main", State: "present"}
	return database.Insert(tx, "files", f, by)
}
