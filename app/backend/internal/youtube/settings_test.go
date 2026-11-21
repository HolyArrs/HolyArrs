package youtube

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// testService is a Service whose folder check accepts every path and whose
// tools are both found.
func testService(s *database.Store) *Service {
	return &Service{Store: s,
		Tools:       func() Tools { return Tools{YTDLP: "/bin/yt-dlp", FFmpeg: "/bin/ffmpeg"} },
		CheckFolder: func(context.Context, string) error { return nil },
		FreeSpace:   func(string) int64 { return 50 << 30 },
	}
}

func TestLoadSettingsDefaults(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	got := LoadSettings(context.Background(), s.Read())
	if got != (Settings{MinFreeGB: 10}) {
		t.Fatalf("defaults %+v", got)
	}
}

func TestSettingsFolderChangeRefusedWithVideos(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	svc := testService(s)
	a, b := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")

	got, err := svc.SaveSettings(ctx, Settings{Folder: a + "/", MinFreeGB: 5}, database.System)
	if err != nil || got.Folder != a || got.MinFreeGB != 5 {
		t.Fatalf("first save: %+v %v", got, err)
	}
	// No downloaded video yet: the folder may move.
	if got, err = svc.SaveSettings(ctx, Settings{Folder: b, MinFreeGB: 5}, database.System); err != nil || got.Folder != b {
		t.Fatalf("move with no videos: %+v %v", got, err)
	}
	if _, err = svc.SaveSettings(ctx, Settings{Folder: a, MinFreeGB: 5}, database.System); err != nil {
		t.Fatal(err)
	}
	// A queued video does not hold the folder; a done one does.
	addVideo(t, s, Video{UserID: 2, VideoID: "aaaaaaaaaaa", State: StateQueued})
	if _, err = svc.SaveSettings(ctx, Settings{Folder: b, MinFreeGB: 5}, database.System); err != nil {
		t.Fatalf("queued only: %v", err)
	}
	if _, err = svc.SaveSettings(ctx, Settings{Folder: a, MinFreeGB: 5}, database.System); err != nil {
		t.Fatal(err)
	}
	addVideo(t, s, Video{UserID: 2, VideoID: "bbbbbbbbbbb", State: StateDone, SizeBytes: 10})
	for _, to := range []string{b, ""} {
		if _, err = svc.SaveSettings(ctx, Settings{Folder: to, MinFreeGB: 5}, database.System); !errors.Is(err, ErrFolderHasVideos) {
			t.Fatalf("move to %q with a done video: %v", to, err)
		}
	}
	if got := LoadSettings(ctx, s.Read()); got.Folder != a {
		t.Fatalf("refused move changed the folder: %+v", got)
	}
	// The same folder with another floor still saves.
	if got, err = svc.SaveSettings(ctx, Settings{Folder: a, MinFreeGB: 20}, database.System); err != nil || got.MinFreeGB != 20 {
		t.Fatalf("floor change: %+v %v", got, err)
	}
}

func TestSettingsChecksFolderAndFloor(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	svc := testService(s)
	bad := errors.New("bad folder")
	var checked []string
	svc.CheckFolder = func(_ context.Context, p string) error {
		checked = append(checked, p)
		if filepath.Base(p) == "bad" {
			return bad
		}
		return nil
	}
	if _, err := svc.SaveSettings(ctx, Settings{Folder: "/x/bad", MinFreeGB: 5}, database.System); !errors.Is(err, bad) {
		t.Fatalf("bad folder: %v", err)
	}
	for _, gb := range []int{0, -1, MaxFreeGB + 1} {
		if _, err := svc.SaveSettings(ctx, Settings{Folder: "/x/good", MinFreeGB: gb}, database.System); !errors.Is(err, ErrMinFree) {
			t.Fatalf("floor %d: %v", gb, err)
		}
	}
	if _, err := svc.SaveSettings(ctx, Settings{Folder: "/x/good", MinFreeGB: 5}, database.System); err != nil {
		t.Fatal(err)
	}
	checked = nil
	// An unchanged folder is not checked again, and no folder needs no check.
	if _, err := svc.SaveSettings(ctx, Settings{Folder: "/x/good", MinFreeGB: 6}, database.System); err != nil || len(checked) != 0 {
		t.Fatalf("unchanged folder checked %v: %v", checked, err)
	}
	if got, err := svc.SaveSettings(ctx, Settings{MinFreeGB: 6}, database.System); err != nil || got.Folder != "" || len(checked) != 0 {
		t.Fatalf("cleared folder: %+v %v %v", got, err, checked)
	}
	// The update fields are never set from outside.
	if got, _ := svc.SaveSettings(ctx, Settings{MinFreeGB: 6, UpdatedAt: "x", UpdateError: "y"}, database.System); got.UpdatedAt != "" || got.UpdateError != "" {
		t.Fatalf("update fields saved: %+v", got)
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	svc := testService(s)
	free := int64(50 << 30)
	svc.FreeSpace = func(string) int64 { return free }

	st := svc.Status(ctx)
	if st.Missing == nil || len(st.Missing) != 0 || st.FolderSet || st.LowSpace || st.FloorBytes != 10<<30 {
		t.Fatalf("no folder: %+v", st)
	}
	if _, err := svc.SaveSettings(ctx, Settings{Folder: "/x/yt", MinFreeGB: 20}, database.System); err != nil {
		t.Fatal(err)
	}
	st = svc.Status(ctx)
	if !st.FolderSet || st.LowSpace || st.FreeBytes != free || st.FloorBytes != 20<<30 {
		t.Fatalf("plenty of space: %+v", st)
	}
	free = 19 << 30
	if st = svc.Status(ctx); !st.LowSpace {
		t.Fatalf("low space: %+v", st)
	}
	free = -1
	if st = svc.Status(ctx); st.LowSpace {
		t.Fatalf("unknown space counts as low: %+v", st)
	}
	svc.Tools = func() Tools { return Tools{} }
	if st = svc.Status(ctx); !slices.Equal(st.Missing, []string{"yt-dlp", "ffmpeg"}) {
		t.Fatalf("missing tools: %+v", st)
	}
}
