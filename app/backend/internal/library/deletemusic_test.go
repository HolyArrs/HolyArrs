package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// adoptedLabBand adopts the lab music folder and adds a found file and an
// imported file, returning the artist.
func adoptedLabBand(t *testing.T) (*Service, mediafiles.LibraryFolder, int64) {
	t.Helper()
	svc, _, _, lf := musicService(t)
	labMusicFolder(t, lf.Path)
	res, err := svc.Adopt(context.Background(), lf.ID, []AdoptChoice{{Folder: "Lab Band", ID: "mbid:" + labArtist}}, domain.System)
	if err != nil || res[0].Error != "" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := svc.Rescan(context.Background(), domain.System); err != nil {
		t.Fatal(err)
	}
	return svc, lf, res[0].TitleID
}

func partOf(t *testing.T, svc *Service, titleID int64, mbid string) int64 {
	t.Helper()
	var id int64
	if err := svc.Store.Read().QueryRow("SELECT id FROM parts WHERE title_id = ? AND json_extract(source_ids, '$.musicbrainz') = ?", titleID, mbid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// 4.15.3: deleting an album keeps files by default; with delete files the
// adopted and imported tracks go to the holding folder, found files stay,
// the plan is exactly what is applied, and the album stops being kept up
// to date while the artist stays.
func TestRemoveAlbum(t *testing.T) {
	svc, lf, id := adoptedLabBand(t)
	ctx := context.Background()
	album := partOf(t, svc, id, rgOld)
	keep, err := svc.RemoveEpisodes(ctx, id, RemoveScope{AlbumID: &album}, RemoveChoice{}, false, domain.System)
	if err != nil || keep.Scope != "album" || keep.Album != "Old Days" || len(keep.Held) != 0 || len(keep.Kept) != 4 || len(keep.Tracks) != 4 {
		t.Fatalf("keep files: %+v %v", keep, err)
	}
	// A found file in the album folder.
	writeFile(t, filepath.Join(lf.Path, "Lab Band", "Old Days (2010)", "extra.mp3"), "x")
	if _, err := svc.Rescan(ctx, domain.System); err != nil {
		t.Fatal(err)
	}
	c := RemoveChoice{DeleteFiles: true}
	dry, err := svc.RemoveEpisodes(ctx, id, RemoveScope{AlbumID: &album}, c, false, domain.System)
	if err != nil || len(dry.Held) != 4 {
		t.Fatalf("dry %+v %v", dry, err)
	}
	if dry.Held[0].Episodes[0] == "" {
		t.Fatalf("no track label: %+v", dry.Held[0])
	}
	done, err := svc.RemoveEpisodes(ctx, id, RemoveScope{AlbumID: &album}, c, true, domain.System)
	if err != nil || !done.Applied || fmt.Sprint(done.Held) != fmt.Sprint(dry.Held) {
		t.Fatalf("apply differs from the plan:\n%+v\n%+v", done.Held, dry.Held)
	}
	for _, h := range done.Held {
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(h.To))); err != nil {
			t.Fatalf("not held: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "Old Days (2010)", "extra.mp3")); err != nil {
		t.Fatal("a found file was touched")
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "Lab Band - Middle (2018)", "01 - Song m1.mp3")); err != nil {
		t.Fatal("another album's file was touched")
	}
	var keepAlbum bool
	_ = svc.Store.Read().QueryRow("SELECT keep_up_to_date FROM parts WHERE id = ?", album).Scan(&keepAlbum)
	var state string
	_ = svc.Store.Read().QueryRow("SELECT state FROM titles WHERE id = ?", id).Scan(&state)
	if keepAlbum || state == "removed" {
		t.Fatalf("album kept %v, artist %s", keepAlbum, state)
	}
}

// Deleting some tracks takes only those; Never-add is for whole artists;
// a track of another artist is refused; a scope of a show is refused.
func TestRemoveTracksAndRefusals(t *testing.T) {
	svc, lf, id := adoptedLabBand(t)
	ctx := context.Background()
	var trackID int64
	_ = svc.Store.Read().QueryRow(`SELECT i.id FROM items i JOIN parts p ON p.id = i.part_id WHERE i.title_id = ? AND i.kind = 'track'
		AND json_extract(p.source_ids, '$.musicbrainz') = ? AND json_extract(i.numbers, '$.track') = 2`, id, rgMid).Scan(&trackID)
	p, err := svc.RemoveEpisodes(ctx, id, RemoveScope{TrackIDs: []int64{trackID}}, RemoveChoice{DeleteFiles: true}, true, domain.System)
	if err != nil || len(p.Held) != 1 || p.Held[0].Path != "Lab Band/Lab Band - Middle (2018)/02 - Song m2.mp3" {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "Lab Band - Middle (2018)", "01 - Song m1.mp3")); err != nil {
		t.Fatal("the other track went")
	}
	album := partOf(t, svc, id, rgOld)
	for _, c := range []struct {
		sc   RemoveScope
		ch   RemoveChoice
		want error
	}{
		{RemoveScope{AlbumID: &album}, RemoveChoice{NeverAdd: true}, ErrNeverAddWholeArt},
		{RemoveScope{TrackIDs: []int64{999999}}, RemoveChoice{}, ErrNoSuchTrack},
		{RemoveScope{AlbumID: new(int64)}, RemoveChoice{}, ErrNoSuchAlbumHere},
		{RemoveScope{EpisodeIDs: []int64{trackID}}, RemoveChoice{}, ErrNotShow},
		{RemoveScope{AlbumID: &album, TrackIDs: []int64{trackID}}, RemoveChoice{}, ErrMixedScope},
	} {
		if _, err := svc.RemoveEpisodes(ctx, id, c.sc, c.ch, false, domain.System); !errors.Is(err, c.want) {
			t.Errorf("%+v: %v, want %v", c.sc, err, c.want)
		}
	}
}

// Deleting the whole artist with files and Never-add: only HolyArrs' own
// files go to the holding folder, found files stay, the artist joins
// Never-add and cannot be added again.
func TestRemoveArtistNeverAdd(t *testing.T) {
	svc, lf, id := adoptedLabBand(t)
	ctx := context.Background()
	c := RemoveChoice{DeleteFiles: true, NeverAdd: true}
	dry, err := svc.Remove(ctx, id, c, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	done, err := svc.Remove(ctx, id, c, true, domain.System)
	if err != nil || fmt.Sprint(done.Held) != fmt.Sprint(dry.Held) || len(done.Held) != 6 {
		t.Fatalf("held %d %v", len(done.Held), err)
	}
	for _, k := range done.Kept {
		if k.Origin != "found" {
			t.Fatalf("kept %+v", k)
		}
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(k.Path))); err != nil {
			t.Fatalf("found file gone: %s", k.Path)
		}
	}
	if on, _ := ArtistOnNeverAdd(ctx, svc.Store.Read(), labArtist); !on {
		t.Fatal("not on Never-add")
	}
	if _, _, err := svc.AddArtist(ctx, "mbid:"+labArtist, ArtistAddOptions{}, domain.System); !errors.Is(err, ErrArtistNeverAdd) {
		t.Fatalf("added again: %v", err)
	}
}

// 4.15 review: an album whose tracks sit in disc folders (CD1, CD2) loses
// its emptied album folder too; the artist folder stays.
func TestRemoveAlbumWithDiscFoldersRemovesAlbumFolder(t *testing.T) {
	svc, lf, id := adoptedLabBand(t)
	album := partOf(t, svc, id, rgOld)
	if _, err := svc.RemoveEpisodes(context.Background(), id, RemoveScope{AlbumID: &album}, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "Old Days (2010)")); !os.IsNotExist(err) {
		t.Fatalf("the emptied album folder stays: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "intro.mp3")); err != nil {
		t.Fatal("the artist folder was touched")
	}
}
