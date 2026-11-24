package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A Sonarr library with many episode files makes a Snapshot past 16 MB; it
// is kept and read back whole.
func TestBigSonarrSnapshotKept(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	s := &Snapshot{Kind: Sonarr, Via: ViaAPI}
	pad := strings.Repeat("x", 60)
	for i := 0; i < 2000; i++ {
		tt := Title{SourceID: int64(i + 1), Kind: "show", Name: fmt.Sprintf("Show %d", i)}
		for f := 0; f < 60; f++ {
			tt.Files = append(tt.Files, TitleFile{Name: fmt.Sprintf("Show %d S01E%02d %s.mkv", i, f, pad), Quality: "WEBDL-1080p",
				Episodes: []OldEpisode{{Season: 1, Number: f + 1, AirDate: "2020-01-01", Title: "Episode " + pad}}})
		}
		s.Titles = append(s.Titles, tt)
	}
	saved, err := Save(ctx, st, 1, s)
	if err != nil {
		t.Fatal(err)
	}
	if plain, _ := json.Marshal(s); len(plain) <= 16<<20 || saved.Size >= len(plain)/4 {
		t.Fatalf("test snapshot is %d bytes of JSON, kept as %d; want past 16 MB kept compressed", len(plain), saved.Size)
	}
	_, got, err := Get(ctx, st, saved.ID)
	if err != nil || len(got.Titles) != 2000 || len(got.Titles[1999].Files) != 60 {
		t.Fatalf("read back: %v", err)
	}
}

// Kept Snapshots together stay under MaxSnapshotsSize; the newest is kept
// even when it alone is over it.
func TestSnapshotsSizeCap(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	old := MaxSnapshotsSize
	defer func() { MaxSnapshotsSize = old }()
	one := &Snapshot{Kind: Sonarr, Via: ViaBackup, Notes: []string{strings.Repeat("n", 1000)}}
	first, _ := Save(ctx, st, 1, one)
	MaxSnapshotsSize = 3 * first.Size
	var last Saved
	for i := 0; i < 5; i++ {
		if last, err = Save(ctx, st, 1, one); err != nil {
			t.Fatal(err)
		}
	}
	if list, _ := List(ctx, st); len(list) != 3 || list[0].ID != last.ID {
		t.Fatalf("kept %d, want the newest 3", len(list))
	}
	MaxSnapshotsSize = 10
	if last, err = Save(ctx, st, 1, one); err != nil {
		t.Fatal(err)
	}
	if list, _ := List(ctx, st); len(list) != 1 || list[0].ID != last.ID {
		t.Fatalf("kept %d, want only the newest", len(list))
	}
}

func TestSlimTitleRow(t *testing.T) {
	c := listServer(t, []byte(`[{"id":1,"title":"A","seasons":[{"seasonNumber":1,"monitored":true,"statistics":{"x":1}}],
		"images":[1],"alternateTitles":[1],"overview":"o","statistics":{},
		"movieFile":{"id":9,"relativePath":"a.mkv","quality":{},"mediaInfo":{"x":1}}}]`))
	rows, err := titleRows(c)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	k := rows[0]
	if len(k) != 4 || len(k["seasons"].([]any)[0].(map[string]any)) != 2 || len(k["moviefile"].(map[string]any)) != 3 {
		t.Fatalf("kept %v", k)
	}
}
