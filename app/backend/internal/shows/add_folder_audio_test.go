package shows

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// A show added into a folder with audio languages for new titles gets them
// on its main copy.
func TestAddShowTakesFolderAudio(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	it := []string{"it"}
	if _, err := library.UpdateFolder(ctx, s, lf.ID, library.FolderChange{NewTitleAudio: &it}, domain.System); err != nil {
		t.Fatal(err)
	}
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := s.Read().QueryRow("SELECT audio_languages FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != `["it"]` {
		t.Fatalf("main copy audio %s, want [\"it\"]", got)
	}
}
