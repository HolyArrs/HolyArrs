package mediaservers

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestPlaybackDetailsPreserveBufferingAndSelectedVersion(t *testing.T) {
	w := newWorld(t)
	title, _ := watchTitles(t, w)
	on := true
	m, err := Add(t.Context(), w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: "http://localhost:32400", Token: tok,
		PathFrom: "/library-mount", PathTo: "/plex-mount", MonitorPlayback: &on}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var wanted int64
	if err := w.s.Write(t.Context(), func(tx *sql.Tx) error {
		var rules int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&rules); err != nil {
			return err
		}
		folder := mediafiles.LibraryFolder{Path: "/library-mount", Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", &folder, domain.System); err != nil {
			return err
		}
		for idx, label := range []string{"1080p", "4K"} {
			version := mediafiles.Version{TitleID: title, Label: label, PickRulesID: rules, LibraryFolderID: folder.ID, FolderName: "Matrix " + label, IsMain: idx == 0}
			if _, err := database.Insert(tx, "versions", &version, domain.System); err != nil {
				return err
			}
			file := mediafiles.File{VersionID: version.ID, RelativePath: "Matrix " + label + "/matrix.mkv",
				SlotIDs: []int64{}, Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
			if _, err := database.Insert(tx, "files", &file, domain.System); err != nil {
				return err
			}
			if idx == 1 {
				wanted = version.ID
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	state, err := playbackStateFor(t.Context(), w.s.Read(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC)
	for idx, player := range []string{"playing", "buffering", "buffering", "buffering", "playing", ""} {
		xml := `<MediaContainer size="0"/>`
		if player != "" {
			xml = fmt.Sprintf(`<MediaContainer size="1"><Video type="movie" ratingKey="18" title="The Matrix" thumb="/library/metadata/18/thumb/2">
				<User id="1" title="Alex"/><Player title="TV" state="%s"/><Session id="s1" bandwidth="9000" location="wan"/>
				<Guid id="tmdb://603"/><Media videoResolution="1080"><Part file="/plex-mount/Matrix 1080p/matrix.mkv"/></Media>
				<Media selected="1" videoResolution="4k" videoCodec="hevc"><Part file="/plex-mount/Matrix 4K/matrix.mkv"/></Media>
				<TranscodeSession videoDecision="transcode" videoCodec="h264" height="1080" audioDecision="copy" subtitleDecision="burn" transcodeHwEncoding="nvenc"/>
			</Video></MediaContainer>`, player)
		}
		sample, err := parsePlayback([]byte(xml))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := savePlaybackSnapshot(t.Context(), w.s, m.ID, state.generation, sample, start.Add(time.Duration(idx)*5*time.Second), idx != 0 && idx != 3); err != nil {
			t.Fatal(err)
		}
	}
	view := readPlaybackForTest(t, w, m, playbackFilter{}, start.Add(25*time.Second))
	if len(view.History) != 1 || len(view.TopViewers) != 1 || len(view.TopTitles) != 1 {
		t.Fatalf("wrong history or ranks: %+v", view)
	}
	row := view.History[0]
	if row.VersionID == nil || *row.VersionID != wanted || row.Version != "4K" || row.FileID == nil || row.Poster == "" {
		t.Fatalf("wrong file, version, or cover: %+v", row)
	}
	d := row.Details
	if d == nil || d.BufferEvents != 1 || d.BufferMS != 5000 || d.SourceVideo != "hevc" || d.OutputHeight != 1080 ||
		d.Hardware != "nvenc" || d.Connection != "wan" || d.BandwidthKbps != 9000 || d.SubtitleDecision != "burn" {
		t.Fatalf("wrong details: %+v", d)
	}
}
