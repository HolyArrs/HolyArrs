package library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// addLabArtist adds the lab artist with its details fetched: albums,
// Pressings, tracks and Slots. It returns the artist and its main Version.
func refreshedLabArtist(t *testing.T, svc *Service) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	id, _, err := svc.AddArtist(ctx, "mbid:"+labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	var vID int64
	if err := svc.Store.Read().QueryRow("SELECT id FROM versions WHERE title_id = ?", id).Scan(&vID); err != nil {
		t.Fatal(err)
	}
	return id, vID
}

// trackSlots maps "<album mbid>:<disc>:<track>" to the track's Slot.
func trackSlots(t *testing.T, s *database.Store, titleID int64) map[string]int64 {
	t.Helper()
	rows, err := s.Read().Query(`SELECT s.id, json_extract(p.source_ids, '$.musicbrainz'), json_extract(i.numbers, '$.disc'), json_extract(i.numbers, '$.track')
		FROM slots s JOIN items i ON i.id = s.item_id JOIN parts p ON p.id = i.part_id WHERE i.title_id = ? AND i.kind = 'track' AND i.dropped_at IS NULL`, titleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var sid int64
		var mb string
		var disc, n int
		if err := rows.Scan(&sid, &mb, &disc, &n); err != nil {
			t.Fatal(err)
		}
		out[fmt.Sprintf("%s:%d:%d", mb, disc, n)] = sid
	}
	return out
}

// putTrack writes a track file and its File row.
func putTrack(t *testing.T, s *database.Store, root string, vID int64, rel, origin string, slotIDs ...int64) mediafiles.File {
	t.Helper()
	writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), "audio:"+rel)
	release := "Lab Band - Old Days (2010) [FLAC]"
	f := mediafiles.File{SlotIDs: slotIDs, VersionID: vID, RelativePath: rel, Size: 10, AddedAt: now, Languages: []string{}, Quality: mediafiles.Quality{Source: "flac"},
		MediaInfo: map[string]any{}, ReleaseName: &release, Origin: origin, PlacedBy: "hardlink", Role: "main", State: "present"}
	if slotIDs == nil {
		f.SlotIDs = []int64{}
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", &f, domain.System); return err }); err != nil {
		t.Fatal(err)
	}
	return f
}

// 4.15.1: track files are renamed by the music patterns (artist folder,
// album folder with year, track name, D-NN on an album of two discs), a
// found file is never renamed, and the preview is exactly what the rename
// does; a second preview is empty.
func TestRenameTracksPreviewMatchesResult(t *testing.T) {
	svc, s, _, lf := musicService(t)
	ctx := context.Background()
	id, vID := refreshedLabArtist(t, svc)
	// The deluxe Pressing of Old Days has two discs.
	var partID, dxID int64
	_ = s.Read().QueryRow("SELECT id FROM parts WHERE title_id = ? AND json_extract(source_ids, '$.musicbrainz') = ?", id, rgOld).Scan(&partID)
	_ = s.Read().QueryRow("SELECT id FROM pressings WHERE mbid = ?", relOldDx).Scan(&dxID)
	if _, err := svc.SetAlbum(ctx, partID, nil, dxID, domain.System); err != nil {
		t.Fatal(err)
	}
	sl := trackSlots(t, s, id)
	a := putTrack(t, s, lf.Path, vID, "Lab Band/old days/track1.flac", "imported", sl[rgOld+":1:1"])
	b := putTrack(t, s, lf.Path, vID, "Lab Band/old days/cd2/x.flac", "adopted", sl[rgOld+":2:1"])
	found := putTrack(t, s, lf.Path, vID, "Lab Band/old days/odd.flac", "found", sl[rgOld+":1:2"])
	mid := putTrack(t, s, lf.Path, vID, "Lab Band/Middle (2018)/01 - Song m1.mp3", "imported", sl[rgMid+":1:1"])
	plan, hash, err := PlanRenameHashed(ctx, s.Read(), []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]string{
		a.ID: "Lab Band/Old Days (2010)/1-01 - Song 1.flac",
		b.ID: "Lab Band/Old Days (2010)/2-01 - Song 4.flac",
	}
	got := map[int64]RenameStep{}
	for _, st := range plan {
		got[st.FileID] = st
	}
	if _, ok := got[mid.ID]; ok {
		t.Fatalf("a well-named file is in the plan: %+v", got[mid.ID])
	}
	if got[found.ID].Skip == "" || got[found.ID].To != "" {
		t.Fatalf("found file: %+v", got[found.ID])
	}
	for fid, to := range want {
		if got[fid].To != to || got[fid].Skip != "" {
			t.Fatalf("step %+v, want %s", got[fid], to)
		}
	}
	jobID, err := svc.Rename(ctx, []int64{id}, nil, hash, domain.System)
	if err != nil || jobID == 0 {
		t.Fatal(err)
	}
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 2 {
		t.Fatalf("renamed %d %v", n, err)
	}
	for fid, to := range want {
		if f := fileRow(t, s, fid); f.RelativePath != to {
			t.Fatalf("db %q, want %q", f.RelativePath, to)
		}
		if _, err := os.Stat(filepath.Join(lf.Path, filepath.FromSlash(to))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "old days", "odd.flac")); err != nil {
		t.Fatal("the found file moved")
	}
	again, _ := PlanRename(ctx, s.Read(), []int64{id})
	for _, st := range again {
		if st.To != "" {
			t.Fatalf("still to rename: %+v", st)
		}
	}

	// New patterns: disc folders, The-last artist words in the track name.
	ns, _ := mediafiles.ReadNaming(ctx, s.Read())
	ns.DiscFolders = true
	ns.AlbumFolderPattern = "{year} - {album}"
	if err := mediafiles.PutNamingSettings(ctx, s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	plan, _, _ = PlanRenameHashed(ctx, s.Read(), []int64{id})
	got = map[int64]RenameStep{}
	for _, st := range plan {
		got[st.FileID] = st
	}
	if got[a.ID].To != "Lab Band/2010 - Old Days/Disc 1/01 - Song 1.flac" || got[b.ID].To != "Lab Band/2010 - Old Days/Disc 2/01 - Song 4.flac" ||
		got[mid.ID].To != "Lab Band/2018 - Middle/01 - Song m1.mp3" {
		t.Fatalf("new patterns: %+v", plan)
	}
}

// A case-only rename of a track works on a case-blind disk too, and a
// plan made before the patterns changed is refused (plan_hash).
func TestRenameTracksCaseOnlyAndPlanHash(t *testing.T) {
	svc, s, _, lf := musicService(t)
	ctx := context.Background()
	id, vID := refreshedLabArtist(t, svc)
	sl := trackSlots(t, s, id)
	var key string
	for k := range sl {
		if strings.HasPrefix(k, rgMid+":1:1") {
			key = k
		}
	}
	f := putTrack(t, s, lf.Path, vID, "Lab Band/middle (2018)/01 - song m1.mp3", "imported", sl[key])
	plan, hash, err := PlanRenameHashed(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 || plan[0].To != "Lab Band/Middle (2018)/01 - Song m1.mp3" {
		t.Fatalf("%+v %v", plan, err)
	}
	ns, _ := mediafiles.ReadNaming(ctx, s.Read())
	ns.TrackPattern = "{track}. {track_title}"
	if err := mediafiles.PutNamingSettings(ctx, s, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, h2, _ := PlanRenameHashed(ctx, s.Read(), []int64{id}); h2 == hash {
		t.Fatal("the plan hash did not change with the patterns")
	}
	ns.TrackPattern = mediafiles.DefaultTrackPattern
	_ = mediafiles.PutNamingSettings(ctx, s, ns, domain.System)
	plan, _, _ = PlanRenameHashed(ctx, s.Read(), []int64{id})
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	if r := fileRow(t, s, f.ID); r.RelativePath != "Lab Band/Middle (2018)/01 - Song m1.mp3" {
		t.Fatalf("db %q", r.RelativePath)
	}
	ents, _ := os.ReadDir(filepath.Join(lf.Path, "Lab Band", "Middle (2018)"))
	if len(ents) != 1 || ents[0].Name() != "01 - Song m1.mp3" {
		t.Fatalf("on disk %v", ents)
	}
}

// A track name that would leave the Library folder is cleaned, never
// followed: a slash in a track title becomes a dash.
func TestRenameTracksStayInside(t *testing.T) {
	svc, s, fm, lf := musicService(t)
	ctx := context.Background()
	ps := fm.pressings[rgMid]
	ps[0].Tracks[0].Name = "../../escape"
	id, vID := refreshedLabArtist(t, svc)
	sl := trackSlots(t, s, id)
	putTrack(t, s, lf.Path, vID, "Lab Band/x/a.mp3", "imported", sl[rgMid+":1:1"])
	plan, err := PlanRename(ctx, s.Read(), []int64{id})
	if err != nil || len(plan) != 1 {
		t.Fatalf("%+v %v", plan, err)
	}
	if plan[0].To != "Lab Band/Middle (2018)/01 - ..-..-escape.mp3" {
		t.Fatalf("to %q", plan[0].To)
	}
}

// 4.15 review: a folder whose name changes only in case is renamed on disk
// too (a case-insensitive disk kept the old case before), and the old
// folder does not stay behind on a case-sensitive one.
func TestRenameFolderCaseOnly(t *testing.T) {
	svc, s, _, lf := musicService(t)
	ctx := context.Background()
	id, vID := refreshedLabArtist(t, svc)
	sl := trackSlots(t, s, id)
	var key string
	for k := range sl {
		if strings.HasPrefix(k, rgMid+":1:1") {
			key = k
		}
	}
	putTrack(t, s, lf.Path, vID, "Lab Band/middle (2018)/01 - Song m1.mp3", "imported", sl[key])
	plan, _ := PlanRename(ctx, s.Read(), []int64{id})
	if n, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil || n != 1 {
		t.Fatalf("renamed %d %v", n, err)
	}
	ents, _ := os.ReadDir(filepath.Join(lf.Path, "Lab Band"))
	if len(ents) != 1 || ents[0].Name() != "Middle (2018)" {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("artist folder holds %q", names)
	}
}

// 4.15 review: a rename that moves every track out of an album folder
// removes the emptied old folder; one still holding another file stays.
func TestRenameRemovesEmptiedFolders(t *testing.T) {
	svc, s, _, lf := musicService(t)
	ctx := context.Background()
	id, vID := refreshedLabArtist(t, svc)
	sl := trackSlots(t, s, id)
	var mid, old string
	for k := range sl {
		if strings.HasPrefix(k, rgMid+":1:1") {
			mid = k
		}
		if strings.HasPrefix(k, rgOld+":1:1") {
			old = k
		}
	}
	putTrack(t, s, lf.Path, vID, "Lab Band/Mid stuff/CD1/01 - Song m1.mp3", "imported", sl[mid])
	putTrack(t, s, lf.Path, vID, "Lab Band/Old stuff/01 - Song 1.flac", "imported", sl[old])
	writeFile(t, filepath.Join(lf.Path, "Lab Band", "Old stuff", "cover.jpg"), "img")
	plan, _ := PlanRename(ctx, s.Read(), []int64{id})
	if _, err := svc.ApplyRename(ctx, plan, 0, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "Mid stuff")); !os.IsNotExist(err) {
		t.Fatalf("the emptied folder stays: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band", "Old stuff", "cover.jpg")); err != nil {
		t.Fatal("a folder with another file was removed")
	}
	if _, err := os.Stat(filepath.Join(lf.Path, "Lab Band")); err != nil {
		t.Fatal("the artist folder went")
	}
}
